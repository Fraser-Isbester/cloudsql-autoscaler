package config

import (
	"sort"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// sortedRegistryNames gives a stable iteration order so failures are reproducible.
func sortedRegistryNames() []string {
	names := make([]string, 0, len(MachineTypeRegistry))
	for n := range MachineTypeRegistry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var _ = Describe("Machine type ladder invariants (registry)", func() {
	It("scales up to the smallest strictly larger type in the same series and tier", func() {
		for _, name := range sortedRegistryNames() {
			cur := MachineTypeRegistry[name]
			var want string
			for _, mt := range MachineTypeRegistry {
				if mt.Series != cur.Series || mt.Tier != cur.Tier || mt.CPU <= cur.CPU {
					continue
				}
				if want == "" || mt.CPU < MachineTypeRegistry[want].CPU {
					want = mt.Name
				}
			}

			got, err := GetNextLargerMachineType(name)
			if want == "" {
				Expect(err).To(HaveOccurred(), "%s is the top of its ladder", name)
				continue
			}
			Expect(err).NotTo(HaveOccurred(), name)
			Expect(got).To(Equal(want), name)
			Expect(MachineTypeRegistry[got].MemoryGB).To(BeNumerically(">=", cur.MemoryGB), name)
		}
	})

	It("scales down to the largest strictly smaller type in the same series and tier", func() {
		for _, name := range sortedRegistryNames() {
			cur := MachineTypeRegistry[name]
			var want string
			for _, mt := range MachineTypeRegistry {
				if mt.Series != cur.Series || mt.Tier != cur.Tier || mt.CPU >= cur.CPU || mt.MemoryGB >= cur.MemoryGB {
					continue
				}
				if want == "" || mt.CPU > MachineTypeRegistry[want].CPU {
					want = mt.Name
				}
			}

			got, err := GetNextSmallerMachineType(name)
			if want == "" {
				Expect(err).To(HaveOccurred(), "%s is the bottom of its ladder", name)
				continue
			}
			Expect(err).NotTo(HaveOccurred(), name)
			Expect(got).To(Equal(want), name)
		}
	})

	It("is deterministic despite map iteration order", func() {
		for _, name := range sortedRegistryNames() {
			up1, _ := GetNextLargerMachineType(name)
			down1, _ := GetNextSmallerMachineType(name)
			for range 25 {
				up, _ := GetNextLargerMachineType(name)
				down, _ := GetNextSmallerMachineType(name)
				Expect(up).To(Equal(up1), name)
				Expect(down).To(Equal(down1), name)
			}
		}
	})

	It("round-trips: one step down then one step up returns to the start", func() {
		for _, name := range sortedRegistryNames() {
			down, err := GetNextSmallerMachineType(name)
			if err != nil {
				continue
			}
			up, err := GetNextLargerMachineType(down)
			Expect(err).NotTo(HaveOccurred(), name)
			Expect(up).To(Equal(name), "down(%s)=%s but up(%s)=%s", name, down, down, up)
		}
	})
})

var _ = Describe("Machine type ladder invariants (custom)", func() {
	// walk follows the ladder until it errors, asserting every rung is valid and
	// monotonic. The step bound catches an infinite ladder.
	walk := func(start string, step func(string) (string, error), grows bool) []string {
		rungs := []string{start}
		cur := start
		for range 500 {
			next, err := step(cur)
			if err != nil {
				return rungs
			}
			prev, err := GetMachineType(cur)
			Expect(err).NotTo(HaveOccurred())
			mt, err := GetMachineType(next)
			Expect(err).NotTo(HaveOccurred(), "%s -> %s produced an unparseable/invalid type", cur, next)
			Expect(mt.Series).To(Equal("custom"))

			memMB := int(mt.MemoryGB * 1024)
			Expect(memMB%256).To(Equal(0), "%s memory must be a multiple of 256 MB", next)
			if grows {
				Expect(mt.CPU >= prev.CPU && mt.MemoryGB >= prev.MemoryGB).To(BeTrue(), "%s -> %s shrank", cur, next)
			} else {
				Expect(mt.CPU <= prev.CPU && mt.MemoryGB <= prev.MemoryGB).To(BeTrue(), "%s -> %s grew", cur, next)
			}
			Expect(next).NotTo(Equal(cur))
			rungs = append(rungs, next)
			cur = next
		}
		Fail("custom ladder from " + start + " did not terminate")
		return nil
	}

	DescribeTable("scaling up yields valid, growing types and terminates at the 96 vCPU ceiling",
		func(start string) {
			rungs := walk(start, GetNextLargerMachineType, true)
			top, err := GetMachineType(rungs[len(rungs)-1])
			Expect(err).NotTo(HaveOccurred())
			Expect(top.CPU).To(Equal(96))
		},
		Entry("from 1 vCPU / 3.75 GB", "db-custom-1-3840"),
		Entry("from 4 vCPU / 16 GB", "db-custom-4-16384"),
		Entry("from a highmem shape", "db-custom-8-49152"),
	)

	DescribeTable("scaling down yields valid, shrinking types and terminates",
		func(start string) {
			rungs := walk(start, GetNextSmallerMachineType, false)
			Expect(len(rungs)).To(BeNumerically(">", 1))
		},
		Entry("from 96 vCPU / 384 GB", "db-custom-96-393216"),
		Entry("from 4 vCPU / 16 GB", "db-custom-4-16384"),
	)
})

var _ = Describe("Custom machine types the Cloud SQL API will accept", func() {
	It("only recommends 1 or an even number of vCPUs", func() {
		cur := "db-custom-96-393216"
		for range 50 {
			next, err := GetNextSmallerMachineType(cur)
			if err != nil {
				return
			}
			mt, _ := GetMachineType(next)
			Expect(mt.CPU == 1 || mt.CPU%2 == 0).To(BeTrue(), "%s -> %s", cur, next)
			cur = next
		}
	})

	It("only recommends 1 or an even number of vCPUs when scaling up", func() {
		cur := "db-custom-1-3840"
		for range 50 {
			next, err := GetNextLargerMachineType(cur)
			if err != nil {
				return
			}
			mt, _ := GetMachineType(next)
			Expect(mt.CPU == 1 || mt.CPU%2 == 0).To(BeTrue(), "%s -> %s", cur, next)
			Expect(mt.CPU).To(BeNumerically("<=", 96), "%s -> %s", cur, next)
			cur = next
		}
	})
})
