package netroller

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	PublicIp  = "1.1.1.1"
	PrivateIp = "8.8.8.8"
)

var _ = Describe("Netroller", func() {
	Describe("netpolInfo", func() {
		var sqlInstance *unstructured.Unstructured

		BeforeEach(func() {
			sqlInstance = &unstructured.Unstructured{}
			sqlInstance.SetName("sql-instance")
			sqlInstance.SetNamespace("namespace")
			sqlInstance.SetUID("uid")
			sqlInstance.SetOwnerReferences([]v1.OwnerReference{
				{
					Kind: "Application",
					Name: "owner",
				},
			})
			err := unstructured.SetNestedField(sqlInstance.Object, PublicIp, "status", "publicIpAddress")
			Expect(err).ToNot(HaveOccurred())
			err = unstructured.SetNestedField(sqlInstance.Object, PrivateIp, "status", "privateIpAddress")
			Expect(err).ToNot(HaveOccurred())
		})

		AssertCorrectPulicIp := func() {
			It("should extract correct public IP", func() {
				netpolInfo, err := netpolInfo(sqlInstance)
				Expect(err).ToNot(HaveOccurred())

				Expect(netpolInfo.PublicIP).To(Equal(PublicIp))
			})
		}
		AssertCorrectPrivateIp := func() {
			It("should extract correct private IP", func() {
				netpolInfo, err := netpolInfo(sqlInstance)
				Expect(err).ToNot(HaveOccurred())

				Expect(netpolInfo.PrivateIP).To(Equal(PrivateIp))
			})
		}

		It("should extract correct owner", func() {
			netpolInfo, err := netpolInfo(sqlInstance)
			Expect(err).ToNot(HaveOccurred())

			Expect(netpolInfo.Owner).To(Equal("owner"))
		})

		When("only public IP is set", func() {
			BeforeEach(func() {
				unstructured.RemoveNestedField(sqlInstance.Object, "status", "privateIpAddress")
			})

			AssertCorrectPulicIp()
		})

		When("only private IP is set", func() {
			BeforeEach(func() {
				unstructured.RemoveNestedField(sqlInstance.Object, "status", "publicIpAddress")
			})

			AssertCorrectPrivateIp()
		})

		When("both private and public IPs are set", func() {
			AssertCorrectPulicIp()
			AssertCorrectPrivateIp()
		})

		When("neither private nor public IP is set", func() {
			BeforeEach(func() {
				unstructured.RemoveNestedField(sqlInstance.Object, "status", "publicIpAddress")
				unstructured.RemoveNestedField(sqlInstance.Object, "status", "privateIpAddress")
			})

			It("should return error", func() {
				_, err := netpolInfo(sqlInstance)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("networkPolicy", func() {
		DescribeTable("should create network policy for",
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
				PublicIP:     PublicIp,
			}),
			Entry("Only Private IP", NetpolInfo{
				InstanceName: "only-private-ip",
				PrivateIP:    PrivateIp,
			}),
			Entry("Both", NetpolInfo{
				InstanceName: "both",
				PublicIP:     PublicIp,
				PrivateIP:    PrivateIp,
			}),
		)
	})
})
