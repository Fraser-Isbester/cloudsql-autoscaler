package daemon

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("validateConfig", func() {
	It("returns an error when cfg is nil", func() {
		Expect(validateConfig(nil, 30*time.Minute, 8080)).To(HaveOccurred())
	})

	It("returns an error when ProjectID is empty", func() {
		cfg := &config.Config{ProjectID: ""}
		Expect(validateConfig(cfg, 30*time.Minute, 8080)).To(HaveOccurred())
	})

	It("returns an error when interval is zero", func() {
		cfg := &config.Config{ProjectID: "test-project"}
		Expect(validateConfig(cfg, 0, 8080)).To(HaveOccurred())
	})

	It("returns an error when interval is negative", func() {
		cfg := &config.Config{ProjectID: "test-project"}
		Expect(validateConfig(cfg, -time.Second, 8080)).To(HaveOccurred())
	})

	It("returns an error when httpPort is zero", func() {
		cfg := &config.Config{ProjectID: "test-project"}
		Expect(validateConfig(cfg, 30*time.Minute, 0)).To(HaveOccurred())
	})

	It("returns an error when httpPort is out of range", func() {
		cfg := &config.Config{ProjectID: "test-project"}
		Expect(validateConfig(cfg, 30*time.Minute, 70000)).To(HaveOccurred())
	})

	It("returns nil for a valid configuration", func() {
		cfg := &config.Config{ProjectID: "test-project"}
		Expect(validateConfig(cfg, 30*time.Minute, 8080)).To(Succeed())
	})
})

var _ = Describe("NewDaemonConfig", func() {
	It("exposes the provided values through the Config interface", func() {
		cfg := &config.Config{ProjectID: "test-project", DryRun: true}

		daemonCfg := NewDaemonConfig(cfg, 30*time.Minute, 8080, true)

		Expect(daemonCfg.GetInterval()).To(Equal(30 * time.Minute))
		Expect(daemonCfg.GetHTTPPort()).To(Equal(8080))
		Expect(daemonCfg.IsMetricsEnabled()).To(BeTrue())
		Expect(daemonCfg.GetProjectID()).To(Equal(cfg.ProjectID))
		Expect(daemonCfg.IsDryRun()).To(Equal(cfg.DryRun))
	})
})
