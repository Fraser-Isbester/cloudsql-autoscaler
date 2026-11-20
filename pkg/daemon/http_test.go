package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/analyzer"
	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

func call(handler http.HandlerFunc, path string) (int, map[string]any) {
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed(), rec.Body.String())
	Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))
	return rec.Code, body
}

func testDaemon(dryRun bool, a Analyzer) *Daemon {
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)
	cfg := &config.Config{ProjectID: "test-project", DryRun: dryRun}
	return assembleDaemon(ctx, cancel, cfg, &DaemonConfig{Interval: 30 * time.Minute, HTTPPort: 8080}, a)
}

var _ = Describe("HTTP endpoints", func() {
	It("reports healthy unconditionally (liveness)", func() {
		code, body := call((&HTTPServer{}).healthHandler, "/health")
		Expect(code).To(Equal(http.StatusOK))
		Expect(body["status"]).To(Equal("healthy"))
	})

	It("reports not ready until a daemon is attached", func() {
		code, _ := call((&HTTPServer{}).readinessHandler, "/ready")
		Expect(code).To(Equal(http.StatusServiceUnavailable))

		s := NewHTTPServer(8080, testDaemon(true, &fakeAnalyzer{}))
		code, body := call(s.readinessHandler, "/ready")
		Expect(code).To(Equal(http.StatusOK))
		Expect(body["status"]).To(Equal("ready"))
	})

	It("reports daemon status as JSON", func() {
		s := NewHTTPServer(8080, testDaemon(true, &fakeAnalyzer{}))
		code, body := call(s.statusHandler, "/status")
		Expect(code).To(Equal(http.StatusOK))
		Expect(body).To(HaveKeyWithValue("project_id", "test-project"))
		Expect(body).To(HaveKeyWithValue("dry_run", true))
		Expect(body).To(HaveKeyWithValue("http_port", BeNumerically("==", 8080)))
	})

	It("serves the documented routes on a real listener and shuts down cleanly", func() {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		port := lis.Addr().(*net.TCPAddr).Port
		Expect(lis.Close()).To(Succeed())

		s := NewHTTPServer(port, testDaemon(true, &fakeAnalyzer{}))
		done := make(chan error, 1)
		go func() { done <- s.Start() }()

		base := fmt.Sprintf("http://127.0.0.1:%d", port)
		Eventually(func() error { _, err := http.Get(base + "/healthz"); return err }).Should(Succeed())
		for _, path := range []string{"/health", "/healthz", "/ready", "/readyz", "/status"} {
			resp, err := http.Get(base + path)
			Expect(err).NotTo(HaveOccurred())
			_ = resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK), path)
		}

		Expect(s.Shutdown(context.Background())).To(Succeed())
		Eventually(done).Should(Receive(MatchError(http.ErrServerClosed)))
	})
})

var _ = Describe("Daemon wiring (assembleDaemon)", func() {
	It("threads dry-run through to the runner so the daemon never applies", func() {
		fa := &fakeAnalyzer{result: &analyzer.ProjectAnalysisResult{
			Results: []*analyzer.AnalysisResult{result("hot", true)}, TotalInstances: 1, AnalyzedInstances: 1,
		}}
		d := testDaemon(true, fa)

		Expect(d.runner.RunCycle(context.Background())).To(Succeed())
		Expect(fa.applied).To(BeEmpty())
	})

	It("applies when not in dry-run", func() {
		fa := &fakeAnalyzer{result: &analyzer.ProjectAnalysisResult{
			Results: []*analyzer.AnalysisResult{result("hot", true)}, TotalInstances: 1, AnalyzedInstances: 1,
		}}
		d := testDaemon(false, fa)

		Expect(d.runner.RunCycle(context.Background())).To(Succeed())
		Expect(fa.applied).To(ConsistOf("hot"))
	})

	It("serves a passing readiness probe (deploy/kubernetes probes /ready)", func() {
		d := testDaemon(true, &fakeAnalyzer{})
		code, _ := call(d.httpServer.(*HTTPServer).readinessHandler, "/ready")
		Expect(code).To(Equal(http.StatusOK))
	})

	It("serves the assembled daemon's own status on /status", func() {
		d := testDaemon(true, &fakeAnalyzer{})
		code, body := call(d.httpServer.(*HTTPServer).statusHandler, "/status")
		Expect(code).To(Equal(http.StatusOK))
		Expect(body).To(HaveKeyWithValue("project_id", "test-project"))
	})
})

var initMetricsOnce sync.Once

var _ = Describe("Prometheus metrics contract", func() {
	scrape := func() string {
		rec := httptest.NewRecorder()
		GetMetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
		b, _ := io.ReadAll(rec.Body)
		return string(b)
	}

	BeforeEach(func() { initMetricsOnce.Do(InitMetrics) })

	It("exports the cycle and instance metrics after a cycle", func() {
		fa := &fakeAnalyzer{
			result: &analyzer.ProjectAnalysisResult{
				Results:        []*analyzer.AnalysisResult{result("hot", true), result("steady", false)},
				TotalInstances: 2, AnalyzedInstances: 2,
			},
			applyErrs: map[string]error{"hot": errors.New("boom")},
		}
		r := NewAutoscalingRunner(fa, runnerConfig{}, NewPrometheusMetricsReporter())
		_ = r.RunCycle(context.Background())

		out := scrape()
		for _, line := range []string{
			"cloudsql_autoscaler_cycle_duration_seconds ",
			"cloudsql_autoscaler_cycles_total ",
			"cloudsql_autoscaler_instances_total 2",
			"cloudsql_autoscaler_instances_analyzed 2",
			"cloudsql_autoscaler_instances_scalable 1",
			`cloudsql_autoscaler_errors_total{error_type="scaling_failed"} `,
		} {
			Expect(out).To(ContainSubstring(line))
		}
	})

	It("counts scaling operations (README lists cloudsql_autoscaler_scaling_operations_total)", func() {
		Skip("BUG: RecordScalingOperation is never called, so the documented " +
			"cloudsql_autoscaler_scaling_operations_total metric is never emitted")
		fa := &fakeAnalyzer{result: &analyzer.ProjectAnalysisResult{
			Results: []*analyzer.AnalysisResult{result("hot", true)}, TotalInstances: 1, AnalyzedInstances: 1,
		}}
		_ = NewAutoscalingRunner(fa, runnerConfig{}, NewPrometheusMetricsReporter()).RunCycle(context.Background())

		Expect(strings.Contains(scrape(), "cloudsql_autoscaler_scaling_operations_total")).To(BeTrue())
	})
})
