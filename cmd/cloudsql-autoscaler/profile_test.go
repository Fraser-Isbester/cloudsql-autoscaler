package main

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/rules"
)

var _ = Describe("buildConfigFromProfile", func() {
	DescribeTable("profiles keep a dead band between the scale-down and scale-up thresholds",
		func(profile string, up, down float64) {
			cfg := buildConfigFromProfile(profile)
			Expect(cfg.ScaleUpThreshold).To(Equal(up))
			Expect(cfg.ScaleDownThreshold).To(Equal(down))
			Expect(cfg.ScaleDownThreshold).To(BeNumerically("<", cfg.ScaleUpThreshold))
		},
		Entry("default", "default", 0.8, 0.5),
		Entry("conservative", "conservative", 0.9, 0.3),
		Entry("aggressive", "aggressive", 0.7, 0.6),
	)

	It("falls back to defaults for an unknown profile", func() {
		Expect(buildConfigFromProfile("yolo")).To(Equal(config.DefaultConfig()))
	})
})

// After a scale-down, the same absolute load lands on a smaller machine, so
// utilization rises by the capacity ratio. If that projected utilization crosses
// the scale-up threshold, the autoscaler oscillates between two sizes, paying a
// restart (Enterprise) or burning the Enterprise Plus interval budget each cycle.
func flapsAfterScaleDown(cfg *config.Config, machineType string) (bool, string) {
	engine := rules.NewEngine(cfg)
	inst := &config.InstanceInfo{MachineType: machineType, Edition: config.EditionEnterprisePlus}
	for p95 := 0.0; p95 <= 100; p95 += 0.5 {
		down, _ := engine.AnalyzeInstance(inst, &config.MetricsSummary{CPUP95: p95, MemoryP95Pct: p95, DataPoints: 100})
		if !down.ShouldScale || down.EstimatedSavings <= 0 {
			continue
		}
		from, _ := config.GetMachineType(machineType)
		to, _ := config.GetMachineType(down.RecommendedType)
		projected := &config.MetricsSummary{
			CPUP95:       p95 * float64(from.CPU) / float64(to.CPU),
			MemoryP95Pct: p95 * from.MemoryGB / to.MemoryGB,
			DataPoints:   100,
		}
		back, _ := engine.AnalyzeInstance(&config.InstanceInfo{MachineType: down.RecommendedType, Edition: config.EditionEnterprisePlus}, projected)
		if back.ShouldScale && back.RecommendedType == machineType {
			return true, fmt.Sprintf("P95 %.1f%% on %s -> %s -> %.1f%% -> back to %s",
				p95, machineType, down.RecommendedType, projected.CPUP95, machineType)
		}
	}
	return false, ""
}

var _ = Describe("Scaling stability (no flapping between adjacent sizes)", func() {
	ladder := []string{"db-n2-standard-4", "db-n2-highmem-8", "db-e2-standard-16", "db-n1-standard-2"}

	It("is stable for the conservative profile", func() {
		for _, mt := range ladder {
			flaps, how := flapsAfterScaleDown(buildConfigFromProfile("conservative"), mt)
			Expect(flaps).To(BeFalse(), "%s", how)
		}
	})

	It("is stable for the default profile", func() {
		for _, mt := range ladder {
			flaps, how := flapsAfterScaleDown(buildConfigFromProfile("default"), mt)
			Expect(flaps).To(BeFalse(), "%s", how)
		}
	})

	It("is stable for the aggressive profile", func() {
		for _, mt := range ladder {
			flaps, how := flapsAfterScaleDown(buildConfigFromProfile("aggressive"), mt)
			Expect(flaps).To(BeFalse(), "%s", how)
		}
	})
})

var _ = Describe("CLI flags", func() {
	It("defaults to dry-run so a bare invocation never mutates instances", func() {
		f := rootCmd.Flags().Lookup("dry-run")
		Expect(f).NotTo(BeNil())
		Expect(f.DefValue).To(Equal("true"))
	})

	It("exposes the --force flag that downtime errors tell the user to pass", func() {
		f := rootCmd.Flags().Lookup("force")
		Expect(f).NotTo(BeNil())
		// Off by default: a downtime-causing scale must be an explicit choice.
		Expect(f.DefValue).To(Equal("false"))
	})

	It("carries --force into the config the analyzer validates against", func() {
		DeferCleanup(func(prev bool) { force = prev }, force)

		force = true
		Expect(configFromFlags().Force).To(BeTrue())
		force = false
		Expect(configFromFlags().Force).To(BeFalse())
	})
})
