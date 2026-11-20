package rules

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("Engine decision boundaries", func() {
	var (
		engine   *Engine
		instance *config.InstanceInfo
	)

	metrics := func(cpu, mem float64) *config.MetricsSummary {
		return &config.MetricsSummary{CPUP95: cpu, MemoryP95Pct: mem, DataPoints: 100}
	}

	BeforeEach(func() {
		engine = NewEngine(config.DefaultConfig()) // up > 80%, down < 50%
		instance = &config.InstanceInfo{MachineType: "db-n2-standard-4", Edition: config.EditionEnterprisePlus}
	})

	DescribeTable("thresholds are strict and use P95",
		func(cpu, mem float64, wantScale bool, wantType string) {
			d, err := engine.AnalyzeInstance(instance, metrics(cpu, mem))
			Expect(err).NotTo(HaveOccurred())
			Expect(d.ShouldScale).To(Equal(wantScale))
			Expect(d.RecommendedType).To(Equal(wantType))
			Expect(d.CurrentType).To(Equal("db-n2-standard-4"))
		},
		Entry("CPU exactly at the up threshold holds", 80.0, 60.0, false, ""),
		Entry("CPU just above the up threshold scales up", 80.01, 60.0, true, "db-n2-standard-8"),
		Entry("memory alone above the up threshold scales up", 10.0, 85.0, true, "db-n2-standard-8"),
		Entry("both exactly at the down threshold hold", 50.0, 50.0, false, ""),
		Entry("both just below the down threshold scale down", 49.99, 49.99, true, "db-n2-standard-2"),
		Entry("low CPU but busy memory does not scale down", 10.0, 70.0, false, ""),
		Entry("low memory but busy CPU does not scale down", 70.0, 10.0, false, ""),
	)

	It("requires at least 10 data points", func() {
		m := metrics(95, 95)
		m.DataPoints = 9
		d, _ := engine.AnalyzeInstance(instance, m)
		Expect(d.ShouldScale).To(BeFalse())

		m.DataPoints = 10
		d, _ = engine.AnalyzeInstance(instance, m)
		Expect(d.ShouldScale).To(BeTrue())
	})

	It("never scales a hot instance down when thresholds are misconfigured to overlap", func() {
		cfg := config.DefaultConfig()
		cfg.ScaleUpThreshold, cfg.ScaleDownThreshold = 0.5, 0.9
		d, err := NewEngine(cfg).AnalyzeInstance(instance, metrics(70, 70))
		Expect(err).NotTo(HaveOccurred())
		Expect(d.RecommendedType).To(Equal("db-n2-standard-8"))
	})

	It("holds at the top of the ladder instead of erroring", func() {
		instance.MachineType = "db-n2-standard-128"
		d, err := engine.AnalyzeInstance(instance, metrics(99, 99))
		Expect(err).NotTo(HaveOccurred())
		Expect(d.ShouldScale).To(BeFalse())
		Expect(d.Reason).To(ContainSubstring("Cannot scale up"))
	})

	It("holds at the bottom of the ladder instead of erroring", func() {
		instance.MachineType = "db-n2-standard-2"
		d, err := engine.AnalyzeInstance(instance, metrics(5, 5))
		Expect(err).NotTo(HaveOccurred())
		Expect(d.ShouldScale).To(BeFalse())
		Expect(d.Reason).To(ContainSubstring("Cannot scale down"))
	})

	It("does not recommend anything for a machine type it cannot place on a ladder", func() {
		instance.MachineType = "db-mystery-9"
		d, err := engine.AnalyzeInstance(instance, metrics(99, 99))
		Expect(err).NotTo(HaveOccurred())
		Expect(d.ShouldScale).To(BeFalse())
		Expect(d.RecommendedType).To(BeEmpty())
	})

	It("reports cost direction consistently with the scale direction", func() {
		up, _ := engine.AnalyzeInstance(instance, metrics(95, 50))
		down, _ := engine.AnalyzeInstance(instance, metrics(10, 10))
		Expect(up.EstimatedSavings).To(BeNumerically("<", 0))
		Expect(down.EstimatedSavings).To(BeNumerically(">", 0))
	})

	Describe("downtime classification", func() {
		It("always expects downtime for Enterprise, even with no scaling history", func() {
			instance.Edition = config.EditionEnterprise
			d, _ := engine.AnalyzeInstance(instance, metrics(95, 50))
			Expect(d.DowntimeExpected).To(BeTrue())
		})

		It("treats an unknown edition as downtime-causing (fail safe)", func() {
			instance.Edition = ""
			d, _ := engine.AnalyzeInstance(instance, metrics(95, 50))
			Expect(d.DowntimeExpected).To(BeTrue())
		})

		DescribeTable("Enterprise Plus honours the 30m upscale / 3h downscale intervals",
			func(sinceLast time.Duration, cpu, mem float64, wantDowntime bool) {
				instance.LastScaledTime = time.Now().Add(-sinceLast)
				d, err := engine.AnalyzeInstance(instance, metrics(cpu, mem))
				Expect(err).NotTo(HaveOccurred())
				Expect(d.ShouldScale).To(BeTrue())
				Expect(d.DowntimeExpected).To(Equal(wantDowntime))
				if wantDowntime {
					Expect(d.DowntimeReason).To(ContainSubstring("Wait"))
				}
			},
			Entry("upscale 20m after last scale", 20*time.Minute, 95.0, 50.0, true),
			Entry("upscale 40m after last scale", 40*time.Minute, 95.0, 50.0, false),
			Entry("downscale 1h after last scale", time.Hour, 10.0, 10.0, true),
			Entry("downscale 2h50m after last scale", 170*time.Minute, 10.0, 10.0, true),
			Entry("downscale 3h10m after last scale", 190*time.Minute, 10.0, 10.0, false),
		)

		It("never lets a downtime-causing decision through validation without force", func() {
			instance.Edition = config.EditionEnterprise
			d, _ := engine.AnalyzeInstance(instance, metrics(95, 50))
			Expect(engine.ValidateScalingDecision(d, false)).To(MatchError(ContainSubstring("downtime")))
			Expect(engine.ValidateScalingDecision(d, true)).To(Succeed())
		})
	})
})

var _ = Describe("GetOptimalScalingWindow (downtime-causing editions)", func() {
	It("schedules a 2h window in the future starting at the quietest hour of day", func() {
		day := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
		data := &config.MetricsData{}
		for h := range 24 {
			data.Timestamps = append(data.Timestamps, day.Add(time.Duration(h)*time.Hour))
			cpu := 60.0
			if h == 4 {
				cpu = 5
			}
			data.CPUUtilization = append(data.CPUUtilization, cpu)
		}

		w := GetOptimalScalingWindow(data, config.GetScalingConstraints(config.EditionEnterprise))

		Expect(w.Start.UTC().Hour()).To(Equal(4))
		Expect(w.Start.After(time.Now())).To(BeTrue())
		Expect(w.Start.Sub(time.Now())).To(BeNumerically("<=", 24*time.Hour))
		Expect(w.Duration).To(Equal(2 * time.Hour))
		Expect(w.End.Sub(w.Start)).To(Equal(2 * time.Hour))
	})

	It("falls back to 02:00 when there are no samples", func() {
		w := GetOptimalScalingWindow(&config.MetricsData{}, config.GetScalingConstraints(config.EditionEnterprise))
		Expect(w.Start.UTC().Hour()).To(Equal(2))
	})
})
