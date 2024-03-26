package netroller

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Netroller", func() {
	Describe("creating network policy", func() {
		DescribeTable("should create network policy for given IP addresses",
			func(netpolInfo NetpolInfo) {
				netpol := networkPolicy(&netpolInfo)

				Expect(netpol).ToNot(BeNil())
				metadata := netpol.ObjectMeta
				Expect(metadata.Annotations).To(HaveKeyWithValue(CreatedByAnnotation, "netroll"))
				Expect(metadata.OwnerReferences).To(HaveLen(1))
				Expect(metadata.OwnerReferences[0].Name).To(Equal(netpolInfo.InstanceName))

				egress := netpol.Spec.Egress
				Expect(egress).To(ContainElement(
					HaveField(
						"To", ContainElement(
							HaveField(
								"IPBlock", HaveField(
									"CIDR", fmt.Sprintf("%s/32", netpolInfo.IP),
								),
							)),
					)))
			},
			Entry("Only Public IP", NetpolInfo{
				InstanceName: "only-public-ip",
				IP:           "8.8.8.8",
			}),
		)
	})
})
