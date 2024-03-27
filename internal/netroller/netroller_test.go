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
				expectedIps := []string{netpolInfo.PublicIP, netpolInfo.PrivateIP}
				for _, ip := range expectedIps {
					if ip == "" {
						continue
					}
					Expect(egress).To(ContainElement(
						HaveField(
							"To", ContainElement(
								HaveField(
									"IPBlock", HaveField(
										"CIDR", fmt.Sprintf("%s/32", ip),
									),
								)),
						)))
				}
			},
			Entry("Only Public IP", NetpolInfo{
				InstanceName: "only-public-ip",
				PublicIP:     "8.8.8.8",
			}),
			Entry("Only Private IP", NetpolInfo{
				InstanceName: "only-private-ip",
				PrivateIP:    "8.8.8.8",
			}),
			Entry("Both", NetpolInfo{
				InstanceName: "both",
				PublicIP:     "1.1.1.1",
				PrivateIP:    "8.8.8.8",
			}),
		)
	})
})
