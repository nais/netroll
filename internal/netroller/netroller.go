package netroller

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const CreatedByAnnotation = "nais.io/created-by"

type Netroller struct {
	log       *logrus.Logger
	k8sClient *kubernetes.Clientset
}

type NetpolInfo struct {
	InstanceUID  types.UID
	InstanceName string
	Namespace    string
	PublicIP     string
	PrivateIP    string
	Owner        string
}

func (ni *NetpolInfo) Name() string {
	return fmt.Sprintf("db-%s-%s", ni.Owner, ni.InstanceName)
}

func (n *Netroller) Add(new any) {
	n.log.Debug("received add event")
	n.ensureNetworkPolicy(new)
}

func (n *Netroller) Update(old any, new any) {
	n.log.Debug("received update event")
	oldInstance := old.(*unstructured.Unstructured)
	if oldInstance.GetDeletionTimestamp() != nil {
		n.log.Infof("resource %s in namespace %s is being deleted, ignoring", oldInstance.GetName(), oldInstance.GetNamespace())
		return
	}
	n.ensureNetworkPolicy(new)
}

func New(log *logrus.Logger, k8sClient *kubernetes.Clientset) *Netroller {
	return &Netroller{
		log:       log,
		k8sClient: k8sClient,
	}
}

func (n *Netroller) ensureNetworkPolicy(v any) {
	sqlInstance := v.(*unstructured.Unstructured)
	n.log.Infof("ensuring networkPolicy for sqlInstance %s in namespace %s", sqlInstance.GetName(), sqlInstance.GetNamespace())

	ctx := context.Background()

	netpol, err := netpolInfo(sqlInstance)

	if err != nil {
		n.log.WithError(err).Debug("failed to get required networkPolicy info, ignoring")
		return
	}

	if err := n.createNetworkPolicy(ctx, netpol); err != nil {
		n.log.WithError(err).Errorf("failed to create networkPolicy '%s'", netpol.Name())
	}

	n.log.Infof("ensured networkPolicy %s in namespace %s", netpol.Name(), netpol.Namespace)
}

func (n *Netroller) createNetworkPolicy(ctx context.Context, ni *NetpolInfo) error {
	api := n.k8sClient.NetworkingV1().NetworkPolicies(ni.Namespace)
	np := networkPolicy(ni)

	_, err := api.Get(ctx, ni.Name(), v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("getting networkPolicy '%s': %w", ni.Name(), err)
	}

	if errors.IsNotFound(err) {
		_, err = api.Create(ctx, np, v1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating networkPolicy %s: %w", ni.Name(), err)
		}
		n.log.Infof("created networkPolicy %s in namespace %s", ni.Name(), ni.Namespace)
		return nil
	} else {
		_, err = api.Update(ctx, np, v1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating networkPolicy %s: %w", ni.Name(), err)
		}
		n.log.Infof("updated networkPolicy %s in namespace %s", ni.Name(), ni.Namespace)
		return nil
	}
}

func networkPolicy(i *NetpolInfo) *networkingv1.NetworkPolicy {
	netpol := &networkingv1.NetworkPolicy{
		ObjectMeta: v1.ObjectMeta{
			Name: fmt.Sprintf("db-%s-%s", i.Owner, i.InstanceName),
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion: "sql.cnrm.cloud.google.com/v1beta1",
					Kind:       "SQLInstance",
					Name:       i.InstanceName,
					UID:        i.InstanceUID,
				},
			},
			Annotations: map[string]string{
				CreatedByAnnotation: "netroll",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": i.Owner,
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}
	for _, p := range []string{i.PublicIP, i.PrivateIP} {
		if p != "" {
			netpol.Spec.Egress = append(netpol.Spec.Egress, networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						IPBlock: &networkingv1.IPBlock{
							CIDR: fmt.Sprintf("%s/32", p),
						},
					},
				},
			})
		}
	}
	return netpol
}

func netpolInfo(sqlInstance *unstructured.Unstructured) (*NetpolInfo, error) {
	o, err := owner(sqlInstance)
	if err != nil {
		return nil, err
	}

	pubIp, pubIpErr := publicIp(sqlInstance)
	privIp, privIpErr := privateIp(sqlInstance)
	if pubIpErr != nil && privIpErr != nil {
		return nil, fmt.Errorf("sqlInstance %s has no IP addresses", sqlInstance.GetName())
	}

	return &NetpolInfo{
		InstanceUID:  sqlInstance.GetUID(),
		InstanceName: sqlInstance.GetName(),
		Namespace:    sqlInstance.GetNamespace(),
		PublicIP:     pubIp,
		PrivateIP:    privIp,
		Owner:        o,
	}, nil
}

func publicIp(instance *unstructured.Unstructured) (string, error) {
	status := instance.Object["status"]
	if status == nil {
		return "", fmt.Errorf("cannot get publicIpAddress, sqlInstance %s has no status", instance.GetName())
	}
	m := status.(map[string]any)
	// check if publicIpAddress is available (on newly created instances, this status field may not be set yet)
	if m["publicIpAddress"] == nil {
		return "", fmt.Errorf("sqlInstance %s has no publicIpAddress", instance.GetName())
	}
	i := m["publicIpAddress"].(string)
	if i == "" {
		return "", fmt.Errorf("sqlInstance %s has empty publicIpAddress", instance.GetName())
	}
	return i, nil
}

func privateIp(instance *unstructured.Unstructured) (string, error) {
	status := instance.Object["status"]
	if status == nil {
		return "", fmt.Errorf("cannot get private IP address, sqlInstance %s has no status", instance.GetName())
	}
	m := status.(map[string]any)
	// check if privateIpAddress is available (on newly created instances, this status field may not be set yet. On instances not in the shared VPC, it will never be set)
	if m["privateIpAddress"] == nil {
		return "", fmt.Errorf("sqlInstance %s has no privateIpAddress", instance.GetName())
	}
	i := m["privateIpAddress"].(string)
	if i == "" {
		return "", fmt.Errorf("sqlInstance %s has empty privateIpAddress", instance.GetName())
	}
	return i, nil
}

func owner(instance *unstructured.Unstructured) (string, error) {
	if instance.GetOwnerReferences() == nil {
		return "", fmt.Errorf("sqlInstance %s has no ownerReference", instance.GetName())
	}

	if len(instance.GetOwnerReferences()) != 1 {
		return "", fmt.Errorf("sqlInstance %s has more than one ownerReference", instance.GetName())
	}

	o := instance.GetOwnerReferences()[0]
	if o.Kind != "Application" && o.Kind != "Naisjob" {
		return "", fmt.Errorf("sqlInstance %s has ownerReference of kind %s, expected Application or Naisjob", instance.GetName(), o.Kind)
	}
	return o.Name, nil
}
