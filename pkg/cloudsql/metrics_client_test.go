package cloudsql

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"

	"github.com/fraser-isbester/cloudsql-autoscaler/internal/fakegcp"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("MetricsClient (against fake Cloud Monitoring API)", func() {
	var (
		ctx  context.Context
		fake *fakegcp.Monitoring
		mc   *MetricsClient
		cfg  *config.Config
		end  time.Time
	)

	BeforeEach(func() {
		ctx = context.Background()
		var err error
		fake, err = fakegcp.NewMonitoring()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(fake.Close)
		mc, err = NewMetricsClient(ctx, testProject, fake.ClientOptions()...)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(mc.Close)
		cfg = config.DefaultConfig()
		end = time.Now().Truncate(time.Minute)
	})

	It("queries each metric for this instance over the configured window and alignment", func() {
		fake.SetSeries(fakegcp.MetricCPU, fakegcp.Constant(end, 5*time.Minute, 3, 0.5))

		_, err := mc.GetInstanceMetrics(ctx, "db1", cfg)
		Expect(err).NotTo(HaveOccurred())

		reqs := fake.Requests()
		Expect(reqs).NotTo(BeEmpty())
		for _, r := range reqs {
			Expect(r.GetName()).To(Equal("projects/" + testProject))
			Expect(r.GetFilter()).To(ContainSubstring(`resource.labels.database_id="` + testProject + `:db1"`))
			Expect(r.GetFilter()).To(ContainSubstring(`resource.type="cloudsql_database"`))
			window := r.GetInterval().GetEndTime().AsTime().Sub(r.GetInterval().GetStartTime().AsTime())
			Expect(window).To(Equal(cfg.MetricsPeriod))
			Expect(r.GetAggregation().GetAlignmentPeriod().AsDuration()).To(Equal(cfg.MetricsInterval))
		}
	})

	It("converts fractions to percentages and bytes to GB, aligned on CPU timestamps in order", func() {
		pts := fakegcp.Constant(end, 5*time.Minute, 3, 0)
		cpu := []fakegcp.Point{{At: pts[2].At, Value: 0.9}, {At: pts[0].At, Value: 0.1}, {At: pts[1].At, Value: 0.5}}
		fake.SetSeries(fakegcp.MetricCPU, cpu)
		fake.SetSeries(fakegcp.MetricMemory, []fakegcp.Point{{At: pts[0].At, Value: 0.25}, {At: pts[1].At, Value: 0.5}, {At: pts[2].At, Value: 0.75}})
		fake.SetSeries(fakegcp.MetricMemoryBytes, []fakegcp.Point{{At: pts[0].At, Value: 2 << 30, Int: true}})
		fake.SetSeries(fakegcp.MetricBackends, []fakegcp.Point{{At: pts[1].At, Value: 42, Int: true}})

		data, err := mc.GetInstanceMetrics(ctx, "db1", cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(data.Timestamps).To(HaveLen(3))
		Expect(data.Timestamps[0].Before(data.Timestamps[1])).To(BeTrue())
		Expect(data.Timestamps[1].Before(data.Timestamps[2])).To(BeTrue())
		Expect(data.CPUUtilization).To(HaveExactElements(
			BeNumerically("~", 10, 1e-9), BeNumerically("~", 50, 1e-9), BeNumerically("~", 90, 1e-9)))
		Expect(data.MemoryPercent).To(HaveExactElements(
			BeNumerically("~", 25, 1e-9), BeNumerically("~", 50, 1e-9), BeNumerically("~", 75, 1e-9)))
		Expect(data.MemoryUsageGB[0]).To(BeNumerically("~", 2, 1e-9))
		Expect(data.Connections).To(Equal([]int{0, 42, 0}))
	})

	It("skips absent memory samples instead of reading them as 0% (#18)", func() {
		pts := fakegcp.Constant(end, 5*time.Minute, 3, 0.5)
		fake.SetSeries(fakegcp.MetricCPU, pts)
		fake.SetSeries(fakegcp.MetricMemory, []fakegcp.Point{{At: pts[1].At, Value: 0.6}})

		data, err := mc.GetInstanceMetrics(ctx, "db1", cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(data.Timestamps).To(HaveLen(3))
		Expect(data.MemoryPercent).To(HaveExactElements(BeNumerically("~", 60, 1e-9)))
		summary := CalculateMetricsSummary(data)
		Expect(summary.DataPoints).To(Equal(3))
		Expect(summary.MemoryDataPoints).To(Equal(1))
		Expect(summary.MemoryAvgPct).To(BeNumerically("~", 60, 1e-9))
	})

	It("fails when CPU utilization cannot be fetched", func() {
		fake.FailMetric(fakegcp.MetricCPU, codes.PermissionDenied)

		_, err := mc.GetInstanceMetrics(ctx, "db1", cfg)

		Expect(err).To(MatchError(ContainSubstring("CPU")))
	})

	It("fails when memory utilization cannot be fetched", func() {
		fake.SetSeries(fakegcp.MetricCPU, fakegcp.Constant(end, 5*time.Minute, 3, 0.5))
		fake.FailMetric(fakegcp.MetricMemory, codes.Internal) // non-retryable; Unavailable would trigger gax backoff

		_, err := mc.GetInstanceMetrics(ctx, "db1", cfg)

		Expect(err).To(MatchError(ContainSubstring("memory")))
	})

	It("tolerates missing optional metrics (memory bytes, connections)", func() {
		fake.SetSeries(fakegcp.MetricCPU, fakegcp.Constant(end, 5*time.Minute, 3, 0.5))
		fake.FailMetric(fakegcp.MetricMemoryBytes, codes.NotFound)
		fake.FailMetric(fakegcp.MetricBackends, codes.NotFound) // e.g. a MySQL instance

		data, err := mc.GetInstanceMetrics(ctx, "db1", cfg)

		Expect(err).NotTo(HaveOccurred())
		Expect(data.Timestamps).To(HaveLen(3))
	})
})
