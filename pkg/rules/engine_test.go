package rules

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("Engine", func() {
	var (
		cfg      *config.Config
		engine   *Engine
		instance *config.InstanceInfo
	)

	BeforeEach(func() {
		cfg = config.DefaultConfig()
		engine = NewEngine(cfg)
		instance = &config.InstanceInfo{
			Name:        "test-instance",
			MachineType: "db-n2-standard-4",
			Edition:     config.EditionEnterprisePlus,
		}
	})

	Describe("AnalyzeInstance", func() {
		Context("when there is insufficient metrics data", func() {
			It("does not recommend scaling", func() {
				metrics := &config.MetricsSummary{DataPoints: 5}

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.ShouldScale).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("Insufficient"))
			})
		})

		Context("when load is high", func() {
			It("recommends scaling up to the next larger machine type", func() {
				metrics := &config.MetricsSummary{
					CPUP95:       95,
					MemoryP95Pct: 60,
					DataPoints:   100,
				}

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.ShouldScale).To(BeTrue())
				Expect(decision.RecommendedType).To(Equal("db-n2-standard-8"))
			})
		})

		Context("when load is low", func() {
			It("recommends scaling down to the next smaller machine type", func() {
				metrics := &config.MetricsSummary{
					CPUP95:       20,
					MemoryP95Pct: 20,
					DataPoints:   100,
				}

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.ShouldScale).To(BeTrue())
				Expect(decision.RecommendedType).To(Equal("db-n2-standard-2"))
			})
		})

		Context("when utilization is within the target range", func() {
			It("does not recommend scaling", func() {
				metrics := &config.MetricsSummary{
					CPUP95:       60,
					MemoryP95Pct: 60,
					DataPoints:   100,
				}

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.ShouldScale).To(BeFalse())
				Expect(decision.Reason).To(ContainSubstring("within target range"))
			})
		})

		Context("with an Enterprise edition instance", func() {
			It("expects downtime for a scaling operation", func() {
				instance.Edition = config.EditionEnterprise
				metrics := &config.MetricsSummary{
					CPUP95:       95,
					MemoryP95Pct: 60,
					DataPoints:   100,
				}

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.ShouldScale).To(BeTrue())
				Expect(decision.DowntimeExpected).To(BeTrue())
			})
		})

		Context("with an Enterprise Plus instance", func() {
			var metrics *config.MetricsSummary

			BeforeEach(func() {
				instance.Edition = config.EditionEnterprisePlus
				metrics = &config.MetricsSummary{
					CPUP95:       95,
					MemoryP95Pct: 60,
					DataPoints:   100,
				}
			})

			It("expects downtime when scaled up too recently", func() {
				instance.LastScaledTime = time.Now()

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.DowntimeExpected).To(BeTrue())
			})

			It("does not expect downtime when the last scale was long ago", func() {
				instance.LastScaledTime = time.Now().Add(-25 * time.Hour)

				decision, err := engine.AnalyzeInstance(instance, metrics)

				Expect(err).NotTo(HaveOccurred())
				Expect(decision.DowntimeExpected).To(BeFalse())
			})
		})
	})

	Describe("ValidateScalingDecision", func() {
		It("returns an error when downtime is expected and not forced", func() {
			decision := &cloudsql.ScalingDecision{
				ShouldScale:      true,
				CurrentType:      "db-n2-standard-4",
				RecommendedType:  "db-n2-standard-8",
				DowntimeExpected: true,
				DowntimeReason:   "requires downtime",
			}

			err := engine.ValidateScalingDecision(decision, false)

			Expect(err).To(HaveOccurred())
		})

		It("returns nil when downtime is expected but forced", func() {
			decision := &cloudsql.ScalingDecision{
				ShouldScale:      true,
				CurrentType:      "db-n2-standard-4",
				RecommendedType:  "db-n2-standard-8",
				DowntimeExpected: true,
				DowntimeReason:   "requires downtime",
			}

			err := engine.ValidateScalingDecision(decision, true)

			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error when the recommended type matches the current type", func() {
			decision := &cloudsql.ScalingDecision{
				ShouldScale:     true,
				CurrentType:     "db-n2-standard-4",
				RecommendedType: "db-n2-standard-4",
			}

			err := engine.ValidateScalingDecision(decision, false)

			Expect(err).To(HaveOccurred())
		})

		It("returns nil when no scaling is required", func() {
			decision := &cloudsql.ScalingDecision{
				ShouldScale: false,
			}

			err := engine.ValidateScalingDecision(decision, false)

			Expect(err).NotTo(HaveOccurred())
		})
	})
})
