package daemon

import (
	"context"
	"errors"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/cloudsql"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

type fakeAnalyzer struct {
	mu        sync.Mutex
	result    *analyzer.ProjectAnalysisResult
	err       error
	panicMsg  string
	applyErrs map[string]error
	applied   []string
}

func (f *fakeAnalyzer) AnalyzeAllInstances(context.Context) (*analyzer.ProjectAnalysisResult, error) {
	if f.panicMsg != "" {
		panic(f.panicMsg)
	}
	return f.result, f.err
}

func (f *fakeAnalyzer) ApplyScaling(_ context.Context, name string, _ *cloudsql.ScalingDecision) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, name)
	return f.applyErrs[name]
}

func (f *fakeAnalyzer) Close() error { return nil }

type metricsSpy struct {
	cycles, durations int
	errs              []string
	total, analyzed   int
	scalable          int
}

func (m *metricsSpy) RecordCycleDuration(time.Duration) { m.durations++ }
func (m *metricsSpy) RecordCycleCompletion()            { m.cycles++ }
func (m *metricsSpy) RecordError(t string)              { m.errs = append(m.errs, t) }
func (m *metricsSpy) RecordInstanceCounts(t, a, s int)  { m.total, m.analyzed, m.scalable = t, a, s }

type runnerConfig struct {
	fakeLoopConfig
	dryRun bool
}

func (c runnerConfig) IsDryRun() bool { return c.dryRun }

func result(name string, scale bool) *analyzer.AnalysisResult {
	return &analyzer.AnalysisResult{
		Instance: &config.InstanceInfo{Name: name},
		Decision: &cloudsql.ScalingDecision{ShouldScale: scale, CurrentType: "db-n2-standard-4", RecommendedType: "db-n2-standard-8"},
	}
}

var _ = Describe("autoscalingRunner.RunCycle", func() {
	var (
		fa      *fakeAnalyzer
		metrics *metricsSpy
	)

	BeforeEach(func() {
		fa = &fakeAnalyzer{
			result: &analyzer.ProjectAnalysisResult{
				Results:           []*analyzer.AnalysisResult{result("hot", true), result("steady", false), result("idle", true)},
				TotalInstances:    4,
				AnalyzedInstances: 3,
			},
			applyErrs: map[string]error{},
		}
		metrics = &metricsSpy{}
	})

	run := func(dryRun bool) error {
		return NewAutoscalingRunner(fa, runnerConfig{dryRun: dryRun}, metrics).RunCycle(context.Background())
	}

	It("never applies scaling in dry-run mode", func() {
		Expect(run(true)).To(Succeed())
		Expect(fa.applied).To(BeEmpty())
		Expect(metrics.scalable).To(Equal(2))
	})

	It("applies only instances whose decision says to scale", func() {
		Expect(run(false)).To(Succeed())
		Expect(fa.applied).To(ConsistOf("hot", "idle"))
	})

	It("records instance counts and exactly one completion per cycle", func() {
		Expect(run(false)).To(Succeed())
		Expect([]int{metrics.total, metrics.analyzed, metrics.scalable}).To(Equal([]int{4, 3, 2}))
		Expect(metrics.cycles).To(Equal(1))
		Expect(metrics.durations).To(Equal(1))
	})

	It("keeps going after one instance fails and reports the failure", func() {
		boom := errors.New("quota exceeded")
		fa.applyErrs["hot"] = boom

		err := run(false)

		Expect(err).To(MatchError(boom))
		Expect(fa.applied).To(ConsistOf("hot", "idle"))
		Expect(metrics.errs).To(Equal([]string{"scaling_failed"}))
	})

	It("applies nothing when analysis fails, and wraps the cause", func() {
		boom := errors.New("list failed")
		fa.err = boom

		err := run(false)

		Expect(err).To(MatchError(boom))
		var de *DaemonError
		Expect(errors.As(err, &de)).To(BeTrue())
		Expect(fa.applied).To(BeEmpty())
		Expect(metrics.errs).To(Equal([]string{"analysis_error"}))
		Expect(metrics.cycles).To(Equal(1))
	})

	It("contains a panic inside a cycle and records it", func() {
		fa.panicMsg = "nil map"

		Expect(func() { _ = run(false) }).NotTo(Panic())
		Expect(metrics.errs).To(ContainElement("panic"))
		Expect(metrics.cycles).To(Equal(1))
	})
})
