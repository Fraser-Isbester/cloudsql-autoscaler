package cloudsql

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCloudSQL(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CloudSQL Suite")
}
