package rules

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("Constraints", func() {
	var cfg *config.Config

	BeforeEach(func() {
		cfg = config.DefaultConfig()
	})

	Describe("CheckScalingConstraints", func() {
		// completeMetrics has enough data points to avoid the low-completeness warning.
		completeMetrics := func() *config.MetricsSummary {
			expected := int(cfg.MetricsPeriod / cfg.MetricsInterval)
			return &config.MetricsSummary{DataPoints: expected}
		}

		It("warns when high availability is enabled", func() {
			instance := &config.InstanceInfo{HighAvailability: true}

			warnings := CheckScalingConstraints(instance, completeMetrics(), cfg)

			Expect(warnings).To(ContainElement(ContainSubstring("high availability")))
		})

		It("warns when backups are enabled", func() {
			instance := &config.InstanceInfo{BackupEnabled: true}

			warnings := CheckScalingConstraints(instance, completeMetrics(), cfg)

			Expect(warnings).To(ContainElement(ContainSubstring("backups enabled")))
		})

		It("warns when the instance was scaled recently", func() {
			instance := &config.InstanceInfo{LastScaledTime: time.Now()}

			warnings := CheckScalingConstraints(instance, completeMetrics(), cfg)

			Expect(warnings).To(ContainElement(ContainSubstring("scaled recently")))
		})

		It("warns when metrics data is incomplete", func() {
			instance := &config.InstanceInfo{}
			metrics := &config.MetricsSummary{DataPoints: 10}

			warnings := CheckScalingConstraints(instance, metrics, cfg)

			Expect(warnings).To(ContainElement(ContainSubstring("Limited metrics data")))
		})
	})

	Describe("EstimateDowntime", func() {
		It("returns a positive duration for an Enterprise instance", func() {
			instance := &config.InstanceInfo{Edition: config.EditionEnterprise}

			downtime := EstimateDowntime(instance, "db-n2-standard-4", "db-n2-standard-8")

			Expect(downtime).To(BeNumerically(">", time.Duration(0)))
		})

		It("returns zero downtime for an Enterprise Plus instance", func() {
			instance := &config.InstanceInfo{Edition: config.EditionEnterprisePlus}

			downtime := EstimateDowntime(instance, "db-n2-standard-4", "db-n2-standard-8")

			Expect(downtime).To(Equal(time.Duration(0)))
		})
	})

	Describe("GetOptimalScalingWindow", func() {
		It("returns a 24h window for Enterprise Plus (no downtime on scale)", func() {
			constraints := config.GetScalingConstraints(config.EditionEnterprisePlus)
			metrics := &config.MetricsData{}

			window := GetOptimalScalingWindow(metrics, constraints)

			Expect(window.Duration).To(Equal(24 * time.Hour))
		})
	})
})
