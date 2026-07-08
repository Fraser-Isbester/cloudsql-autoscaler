package cloudsql

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/fraser-isbester/cloudsql-autoscaler/pkg/config"
)

var _ = Describe("EstimateCostSavings", func() {
	It("returns positive savings when scaling down to a smaller machine type", func() {
		savings := EstimateCostSavings("db-n2-standard-4", "db-n2-standard-2", "us-central1")
		Expect(savings).To(BeNumerically(">", 0))
	})

	It("returns negative savings when scaling up to a larger machine type", func() {
		savings := EstimateCostSavings("db-n2-standard-2", "db-n2-standard-4", "us-central1")
		Expect(savings).To(BeNumerically("<", 0))
	})

	It("is symmetric: up-scaling cost equals the negation of down-scaling savings", func() {
		down := EstimateCostSavings("db-n2-standard-4", "db-n2-standard-2", "us-central1")
		up := EstimateCostSavings("db-n2-standard-2", "db-n2-standard-4", "us-central1")
		Expect(up).To(BeNumerically("~", -down, 1e-9))
	})

	It("returns zero savings when the machine type is unchanged", func() {
		savings := EstimateCostSavings("db-n2-standard-4", "db-n2-standard-4", "us-central1")
		Expect(savings).To(BeNumerically("~", 0, 1e-9))
	})
})

var _ = Describe("ValidateScaling", func() {
	newRunnableInstance := func(machineType string) *config.InstanceInfo {
		return &config.InstanceInfo{
			Name:        "test-instance",
			MachineType: machineType,
			State:       "RUNNABLE",
		}
	}

	It("errors when the target machine type equals the current type", func() {
		instance := newRunnableInstance("db-n2-standard-4")
		err := ValidateScaling(instance, "db-n2-standard-4")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("same as current"))
	})

	It("errors when the machine series would change", func() {
		instance := newRunnableInstance("db-n2-standard-4")
		err := ValidateScaling(instance, "db-e2-standard-4")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("cannot change machine series"))
	})

	It("errors when the instance is not in RUNNABLE state", func() {
		instance := newRunnableInstance("db-n2-standard-4")
		instance.State = "STOPPED"
		err := ValidateScaling(instance, "db-n2-standard-2")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("RUNNABLE"))
	})

	It("succeeds for a valid same-series change on a RUNNABLE instance", func() {
		instance := newRunnableInstance("db-n2-standard-4")
		err := ValidateScaling(instance, "db-n2-standard-2")
		Expect(err).ToNot(HaveOccurred())
	})
})
