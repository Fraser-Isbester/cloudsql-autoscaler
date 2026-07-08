package cloudsql

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("CalculateMetricsSummary", func() {
	Context("with empty metrics data", func() {
		It("reports zero data points and zero period", func() {
			summary := CalculateMetricsSummary(&config.MetricsData{})
			Expect(summary.DataPoints).To(Equal(0))
			Expect(summary.Period).To(Equal(time.Duration(0)))
		})
	})

	Context("with a small known series", func() {
		var summary *config.MetricsSummary
		var start time.Time

		BeforeEach(func() {
			start = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			timestamps := make([]time.Time, 5)
			for i := range 5 {
				timestamps[i] = start.Add(time.Duration(i) * 5 * time.Minute)
			}
			data := &config.MetricsData{
				Timestamps:     timestamps,
				CPUUtilization: []float64{10, 20, 30, 40, 50},
			}
			summary = CalculateMetricsSummary(data)
		})

		It("counts all data points", func() {
			Expect(summary.DataPoints).To(Equal(5))
		})

		It("computes the CPU average", func() {
			Expect(summary.CPUAvg).To(BeNumerically("~", 30, 1e-9))
		})

		It("computes the CPU maximum", func() {
			Expect(summary.CPUMax).To(BeNumerically("~", 50, 1e-9))
		})

		It("computes the CPU 95th percentile within (40, 50]", func() {
			Expect(summary.CPUP95).To(BeNumerically(">", 40))
			Expect(summary.CPUP95).To(BeNumerically("<=", 50))
		})

		It("computes the period as last minus first timestamp", func() {
			Expect(summary.Period).To(Equal(20 * time.Minute))
		})
	})
})
