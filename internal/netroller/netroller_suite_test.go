package netroller_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNetroller(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Netroller Suite")
}
