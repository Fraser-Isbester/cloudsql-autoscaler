package config

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetMachineType", func() {
	Context("registry lookups", func() {
		It("returns the registered machine type for a known name", func() {
			mt, err := GetMachineType("db-n2-standard-4")
			Expect(err).NotTo(HaveOccurred())
			Expect(mt.CPU).To(Equal(4))
			Expect(mt.MemoryGB).To(Equal(16.0))
			Expect(mt.Series).To(Equal("n2"))
			Expect(mt.Tier).To(Equal("standard"))
		})

		It("returns an error for an unknown machine type", func() {
			_, err := GetMachineType("db-does-not-exist")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("custom machine types", func() {
		It("parses a valid custom machine type", func() {
			mt, err := GetMachineType("db-custom-4-16384")
			Expect(err).NotTo(HaveOccurred())
			Expect(mt.CPU).To(Equal(4))
			Expect(mt.MemoryGB).To(Equal(16.0))
			Expect(mt.Series).To(Equal("custom"))
		})

		It("rejects a custom type with CPU out of the 1..96 range", func() {
			_, err := GetMachineType("db-custom-100-16384")
			Expect(err).To(HaveOccurred())
		})

		It("rejects a custom type whose memory ratio is too low", func() {
			// 4 vCPUs requires at least 3.6 GB; 1024 MB = 1 GB is below the minimum.
			_, err := GetMachineType("db-custom-4-1024")
			Expect(err).To(HaveOccurred())
		})

		It("rejects a custom type with an invalid format", func() {
			_, err := GetMachineType("db-custom-4")
			Expect(err).To(HaveOccurred())
		})
	})

	Context("performance-optimized machine types", func() {
		It("parses a valid perf-optimized machine type", func() {
			mt, err := GetMachineType("db-perf-optimized-N-2")
			Expect(err).NotTo(HaveOccurred())
			Expect(mt.CPU).To(Equal(2))
			Expect(mt.MemoryGB).To(Equal(16.0))
			Expect(mt.Series).To(Equal("perf-optimized"))
			Expect(mt.Tier).To(Equal("performance"))
		})

		It("returns an error for an unknown perf-optimized suffix", func() {
			_, err := GetMachineType("db-perf-optimized-N-99")
			Expect(err).To(HaveOccurred())
		})
	})
})

var _ = Describe("parseCustomMachineType", func() {
	It("derives the highmem tier when memory per CPU exceeds 4 GB", func() {
		mt, err := parseCustomMachineType("db-custom-4-20480") // 20 GB => 5 GB/vCPU
		Expect(err).NotTo(HaveOccurred())
		Expect(mt.Tier).To(Equal("highmem"))
	})

	It("rejects a non-custom name", func() {
		_, err := parseCustomMachineType("db-n2-standard-4")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetNextLargerMachineType", func() {
	It("returns the next larger registry type in the same series/tier", func() {
		next, err := GetNextLargerMachineType("db-n2-standard-2")
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(Equal("db-n2-standard-4"))
	})

	It("advances perf-optimized types up the sequence", func() {
		next, err := GetNextLargerMachineType("db-perf-optimized-N-2")
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(Equal("db-perf-optimized-N-4"))
	})

	It("errors when already at the largest perf-optimized size", func() {
		_, err := GetNextLargerMachineType("db-perf-optimized-N-16")
		Expect(err).To(HaveOccurred())
	})

	It("returns a different, valid custom type when scaling a custom machine up", func() {
		next, err := GetNextLargerMachineType("db-custom-4-16384")
		Expect(err).NotTo(HaveOccurred())
		Expect(next).NotTo(Equal("db-custom-4-16384"))
		Expect(next).To(HavePrefix("db-custom-"))

		// The returned custom type must itself be parseable/valid.
		mt, err := GetMachineType(next)
		Expect(err).NotTo(HaveOccurred())
		Expect(mt.Series).To(Equal("custom"))
	})

	It("propagates errors for unknown machine types", func() {
		_, err := GetNextLargerMachineType("db-nope")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("GetNextSmallerMachineType", func() {
	It("returns the next smaller registry type in the same series/tier", func() {
		next, err := GetNextSmallerMachineType("db-n2-standard-4")
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(Equal("db-n2-standard-2"))
	})

	It("errors when nothing smaller exists in the series/tier", func() {
		_, err := GetNextSmallerMachineType("db-n2-standard-2")
		Expect(err).To(HaveOccurred())
	})

	It("steps perf-optimized types down the sequence", func() {
		next, err := GetNextSmallerMachineType("db-perf-optimized-N-4")
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(Equal("db-perf-optimized-N-2"))
	})

	It("errors when already at the smallest perf-optimized size", func() {
		_, err := GetNextSmallerMachineType("db-perf-optimized-N-2")
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("MachineTypeRegistry", func() {
	It("uses names that match their map keys", func() {
		for key, mt := range MachineTypeRegistry {
			Expect(mt.Name).To(Equal(key))
			Expect(strings.HasPrefix(key, "db-")).To(BeTrue())
		}
	})
})
