package config

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetScalingConstraints", func() {
	It("marks Enterprise as causing downtime on scale", func() {
		c := GetScalingConstraints(EditionEnterprise)
		Expect(c.DowntimeOnScale).To(BeTrue())
	})

	It("gives Enterprise Plus near-zero downtime with tighter intervals", func() {
		c := GetScalingConstraints(EditionEnterprisePlus)
		Expect(c.DowntimeOnScale).To(BeFalse())
		Expect(c.MinUpscaleInterval).To(Equal("30m"))
		Expect(c.MinDownscaleInterval).To(Equal("3h"))
	})

	It("falls back to restrictive defaults for an unknown edition", func() {
		c := GetScalingConstraints(Edition("SOMETHING_ELSE"))
		Expect(c.DowntimeOnScale).To(BeTrue())
		Expect(c.MinUpscaleInterval).To(Equal("24h"))
		Expect(c.MinDownscaleInterval).To(Equal("24h"))
	})
})

var _ = Describe("ParseEdition", func() {
	It("parses enterprise_plus regardless of case", func() {
		Expect(ParseEdition("enterprise_plus")).To(Equal(EditionEnterprisePlus))
		Expect(ParseEdition("ENTERPRISE_PLUS")).To(Equal(EditionEnterprisePlus))
	})

	It("parses enterprise", func() {
		Expect(ParseEdition("enterprise")).To(Equal(EditionEnterprise))
	})

	It("defaults unknown values to Enterprise", func() {
		Expect(ParseEdition("mystery")).To(Equal(EditionEnterprise))
	})
})

var _ = Describe("DefaultConfig", func() {
	It("returns sane default scaling thresholds and intervals", func() {
		c := DefaultConfig()
		Expect(c.ScaleUpThreshold).To(Equal(0.8))
		Expect(c.ScaleDownThreshold).To(Equal(0.5))
		Expect(c.MetricsInterval).To(Equal(5 * time.Minute))
	})
})
