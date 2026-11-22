package analyzer

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sqladmin "google.golang.org/api/sqladmin/v1"
	"google.golang.org/grpc/codes"

	"github.com/fraser-isbester/cloudsql-autoscaler/internal/fakegcp"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

const project = "test-project"

// fixture wires a real Analyzer to fake Cloud SQL Admin and Monitoring backends.
// The fake monitoring backend serves the same series for every instance, so
// per-instance load is modeled by giving each test its own instance mix.
type fixture struct {
	sql *fakegcp.SQLAdmin
	mon *fakegcp.Monitoring
	cfg *config.Config
	pa  *ProjectAnalyzer
}

func newFixture(mutate func(*config.Config)) *fixture {
	ctx := context.Background()
	f := &fixture{sql: fakegcp.NewSQLAdmin(), cfg: config.DefaultConfig()}
	DeferCleanup(f.sql.Close)
	var err error
	f.mon, err = fakegcp.NewMonitoring()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(f.mon.Close)

	f.cfg.ProjectID = project
	if mutate != nil {
		mutate(f.cfg)
	}
	sc, err := cloudsql.NewClient(ctx, project, f.sql.ClientOptions()...)
	Expect(err).NotTo(HaveOccurred())
	mc, err := cloudsql.NewMetricsClient(ctx, project, f.mon.ClientOptions()...)
	Expect(err).NotTo(HaveOccurred())
	f.pa = &ProjectAnalyzer{Analyzer: newAnalyzer(f.cfg, sc, mc)}
	DeferCleanup(f.pa.Close)
	return f
}

// load serves a full window (no completeness warning) of constant utilization.
func (f *fixture) load(cpuFrac, memFrac float64) {
	n := int(f.cfg.MetricsPeriod / f.cfg.MetricsInterval)
	end := time.Now().Truncate(f.cfg.MetricsInterval)
	f.mon.SetSeries(fakegcp.MetricCPU, fakegcp.Constant(end, f.cfg.MetricsInterval, n, cpuFrac))
	f.mon.SetSeries(fakegcp.MetricMemory, fakegcp.Constant(end, f.cfg.MetricsInterval, n, memFrac))
}

func (f *fixture) instance(name, tier, edition string) {
	f.sql.AddInstance(&sqladmin.DatabaseInstance{
		Name: name, State: "RUNNABLE", Region: "us-central1",
		Settings: &sqladmin.Settings{
			Tier: tier, Edition: edition, AvailabilityType: "ZONAL",
			BackupConfiguration: &sqladmin.BackupConfiguration{},
		},
	})
}

var _ = Describe("Analyzer end to end (fake GCP)", func() {
	ctx := context.Background()

	Describe("AnalyzeInstance", func() {
		It("recommends scaling a hot Enterprise Plus instance up with no downtime", func() {
			f := newFixture(nil)
			f.instance("hot", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.load(0.95, 0.60)

			r, err := f.pa.AnalyzeInstance(ctx, "hot")

			Expect(err).NotTo(HaveOccurred())
			Expect(r.Summary.DataPoints).To(Equal(int(f.cfg.MetricsPeriod / f.cfg.MetricsInterval)))
			Expect(r.Summary.CPUP95).To(BeNumerically("~", 95, 1e-6))
			Expect(r.Decision.ShouldScale).To(BeTrue())
			Expect(r.Decision.RecommendedType).To(Equal("db-n2-standard-8"))
			Expect(r.Decision.DowntimeExpected).To(BeFalse())
			Expect(r.ScalingWindow).NotTo(BeNil())
			Expect(r.Warnings).NotTo(ContainElement(ContainSubstring("Limited metrics data")))
		})

		It("holds a steady instance", func() {
			f := newFixture(nil)
			f.instance("steady", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.load(0.65, 0.65)

			r, err := f.pa.AnalyzeInstance(ctx, "steady")

			Expect(err).NotTo(HaveOccurred())
			Expect(r.Decision.ShouldScale).To(BeFalse())
			Expect(r.ScalingWindow).To(BeNil())
		})

		It("treats a recent completed UPDATE as the last scale for Enterprise Plus interval checks", func() {
			f := newFixture(nil)
			f.instance("hot", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.load(0.95, 0.60)
			f.sql.AddOperation(&sqladmin.Operation{
				TargetId: "hot", OperationType: "UPDATE", Status: "DONE",
				InsertTime: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
			})

			r, err := f.pa.AnalyzeInstance(ctx, "hot")

			Expect(err).NotTo(HaveOccurred())
			Expect(r.Decision.ShouldScale).To(BeTrue())
			Expect(r.Decision.DowntimeExpected).To(BeTrue())
			Expect(r.Warnings).To(ContainElement(ContainSubstring("scaled recently")))
		})

		It("fails rather than guessing when CPU metrics are unavailable", func() {
			f := newFixture(nil)
			f.instance("db", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.mon.FailMetric(fakegcp.MetricCPU, codes.PermissionDenied)

			_, err := f.pa.AnalyzeInstance(ctx, "db")

			Expect(err).To(HaveOccurred())
		})

		It("does not recommend a scale-down when no CPU samples exist", func() {
			f := newFixture(nil)
			f.instance("new", "db-n2-standard-4", "ENTERPRISE_PLUS")

			r, err := f.pa.AnalyzeInstance(ctx, "new")

			Expect(err).NotTo(HaveOccurred())
			Expect(r.Decision.ShouldScale).To(BeFalse())
		})

		It("does not read missing memory samples as an idle instance", func() {
			Skip("BUG: GetInstanceMetrics fills absent memory samples with 0%, so an instance whose memory " +
				"series is empty (or gappy) looks idle and gets scaled down on CPU alone")
			f := newFixture(nil)
			f.instance("db", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.load(0.10, 0)
			f.mon.SetSeries(fakegcp.MetricMemory, nil)

			r, err := f.pa.AnalyzeInstance(ctx, "db")

			Expect(err).NotTo(HaveOccurred())
			Expect(r.Decision.ShouldScale).To(BeFalse())
		})
	})

	Describe("ApplyScaling (the only path that mutates instances)", func() {
		hotDecision := func(f *fixture, name, edition string) *cloudsql.ScalingDecision {
			f.instance(name, "db-n2-standard-4", edition)
			f.load(0.95, 0.60)
			r, err := f.pa.AnalyzeInstance(ctx, name)
			Expect(err).NotTo(HaveOccurred())
			Expect(r.Decision.ShouldScale).To(BeTrue())
			return r.Decision
		}

		It("never calls the API in dry-run mode", func() {
			f := newFixture(func(c *config.Config) { c.DryRun = true })
			d := hotDecision(f, "hot", "ENTERPRISE_PLUS")

			Expect(f.pa.ApplyScaling(ctx, "hot", d)).To(Succeed())
			Expect(f.sql.Updates()).To(BeEmpty())
		})

		It("updates the tier to the recommended type when live", func() {
			f := newFixture(nil)
			d := hotDecision(f, "hot", "ENTERPRISE_PLUS")

			Expect(f.pa.ApplyScaling(ctx, "hot", d)).To(Succeed())

			updates := f.sql.Updates()
			Expect(updates).To(HaveLen(1))
			Expect(updates[0].Instance).To(Equal("hot"))
			Expect(updates[0].Body.Settings.Tier).To(Equal("db-n2-standard-8"))
		})

		It("refuses a downtime-causing change without force and does not touch the instance", func() {
			f := newFixture(nil)
			d := hotDecision(f, "ent", "ENTERPRISE")
			Expect(d.DowntimeExpected).To(BeTrue())

			Expect(f.pa.ApplyScaling(ctx, "ent", d)).To(MatchError(ContainSubstring("downtime")))
			Expect(f.sql.Updates()).To(BeEmpty())
		})

		It("applies a downtime-causing change when forced", func() {
			f := newFixture(func(c *config.Config) { c.Force = true })
			d := hotDecision(f, "ent", "ENTERPRISE")

			Expect(f.pa.ApplyScaling(ctx, "ent", d)).To(Succeed())
			Expect(f.sql.Updates()).To(HaveLen(1))
		})

		It("rejects a no-op decision", func() {
			f := newFixture(nil)
			f.instance("db", "db-n2-standard-4", "ENTERPRISE_PLUS")

			err := f.pa.ApplyScaling(ctx, "db", &cloudsql.ScalingDecision{ShouldScale: false})

			Expect(err).To(HaveOccurred())
			Expect(f.sql.Updates()).To(BeEmpty())
		})

		It("surfaces a failed long-running operation", func() {
			f := newFixture(nil)
			d := hotDecision(f, "hot", "ENTERPRISE_PLUS")
			f.sql.SetOperationResult(&sqladmin.Operation{
				Status: "DONE",
				Error:  &sqladmin.OperationErrors{Errors: []*sqladmin.OperationError{{Code: "INTERNAL"}}},
			})

			Expect(f.pa.ApplyScaling(ctx, "hot", d)).To(HaveOccurred())
		})
	})

	Describe("AnalyzeAllInstances", func() {
		It("returns an empty result for an empty project", func() {
			f := newFixture(nil)

			res, err := f.pa.AnalyzeAllInstances(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(res.Results).To(BeEmpty())
			Expect(res.GetScalableInstances()).To(BeEmpty())
		})

		It("counts every instance but analyzes only those it understands", func() {
			f := newFixture(nil)
			f.instance("a", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.instance("b", "db-e2-standard-8", "ENTERPRISE")
			f.instance("weird", "db-mystery-9", "ENTERPRISE")
			f.load(0.95, 0.60)

			res, err := f.pa.AnalyzeAllInstances(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(res.TotalInstances).To(Equal(3))
			Expect(res.AnalyzedInstances).To(Equal(2))
			Expect(res.GetScalableInstances()).To(HaveLen(2))
		})

		It("counts and analyzes instances across every page of the list", func() {
			f := newFixture(nil)
			f.sql.ListPageSize = 2
			f.instance("a", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.instance("b", "db-e2-standard-8", "ENTERPRISE")
			f.instance("c", "db-n2-standard-4", "ENTERPRISE")
			f.instance("d", "db-n2-standard-4", "ENTERPRISE")
			f.instance("weird", "db-mystery-9", "ENTERPRISE")
			f.load(0.95, 0.60)

			res, err := f.pa.AnalyzeAllInstances(ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(res.TotalInstances).To(Equal(5))
			Expect(res.AnalyzedInstances).To(Equal(4))
		})

		It("orders the scaling plan so zero-downtime changes go first", func() {
			f := newFixture(nil)
			f.instance("ent", "db-n2-standard-4", "ENTERPRISE")
			f.instance("plus", "db-n2-standard-4", "ENTERPRISE_PLUS")
			f.load(0.95, 0.60)

			res, err := f.pa.AnalyzeAllInstances(ctx)
			Expect(err).NotTo(HaveOccurred())
			plan := res.GenerateScalingPlan()

			Expect(plan.Operations).To(HaveLen(2))
			Expect(plan.Operations[0].Instance).To(Equal("plus"))
			Expect(plan.Operations[0].Priority).To(BeNumerically(">", plan.Operations[1].Priority))
		})

		It("fails the cycle when the instance list cannot be read", func() {
			f := newFixture(nil)
			f.sql.FailWith("GET", "/projects/"+project+"/instances", 403)

			_, err := f.pa.AnalyzeAllInstances(ctx)

			Expect(err).To(HaveOccurred())
		})
	})
})
