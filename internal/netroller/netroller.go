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

type Netroller struct {
	log       *logrus.Logger
	k8sClient *kubernetes.Clientset
}

type NetpolInfo struct {
	InstanceUID  types.UID
	InstanceName string
	Namespace    string
	IP           string
	Owner        string
}

func (ni *NetpolInfo) Name() string {
	return fmt.Sprintf("db-%s-%s", ni.Owner, ni.InstanceName)
}

func (n *Netroller) Add(new any) {
	n.log.Debugf("update")
	if err := n.ensureNetworkPolicy(new); err != nil {
		n.log.Errorf("uhoh")
	}
}

func (n *Netroller) Update(old any, new any) {
	n.log.Debugf("update")
	if err := n.ensureNetworkPolicy(new); err != nil {
		n.log.Errorf("uhoh")
	}
}

func New(log *logrus.Logger, k8sClient *kubernetes.Clientset) *Netroller {
	return &Netroller{
		log:       log,
		k8sClient: k8sClient,
	}
}

func (n *Netroller) ensureNetworkPolicy(v any) error {
	sqlInstance := v.(*unstructured.Unstructured)
	n.log.Debugf("ensuring networkPolicy for sqlInstance %s", sqlInstance.GetName())

	ctx := context.Background()

	netpol, err := n.netpolInfo(sqlInstance)
	if err != nil {
		n.log.WithError(err).Debug("failed to get required networkPolicy info, ignoring")
		return nil
	}

	if err := n.createNetworkPolicy(ctx, netpol); err != nil {
		n.log.WithError(err).Errorf("failed to create networkPolicy %s", netpol.Name())
	}

	n.log.Debugf("ensured networkPolicy %s", netpol.Name())
	return nil
}

func (n *Netroller) netpolInfo(sqlInstance *unstructured.Unstructured) (*NetpolInfo, error) {
	o, err := owner(sqlInstance)
	if err != nil {
		return nil, err
	}

	i, err := ip(sqlInstance)
	if err != nil {
		return nil, err
	}

	return &NetpolInfo{
		InstanceUID:  sqlInstance.GetUID(),
		InstanceName: sqlInstance.GetName(),
		Namespace:    sqlInstance.GetNamespace(),
		IP:           i,
		Owner:        o,
	}, nil
}

func (n *Netroller) createNetworkPolicy(ctx context.Context, ni *NetpolInfo) error {
	api := n.k8sClient.NetworkingV1().NetworkPolicies(ni.Namespace)
	np := networkPolicy(ni)

	_, err := api.Get(ctx, ni.Name(), v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("getting networkPolicy %s: %w", ni.Name(), err)
	}

	if errors.IsNotFound(err) {
		_, err = api.Create(ctx, np, v1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("creating networkPolicy %s: %w", ni.Name(), err)
		}
		return nil
	} else {
		_, err = api.Update(ctx, np, v1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("updating networkPolicy %s: %w", ni.Name(), err)
		}
		return nil
	}
}

func networkPolicy(i *NetpolInfo) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
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
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: v1.LabelSelector{
				MatchLabels: map[string]string{
					"app": i.Owner,
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: fmt.Sprintf("%s/32", i.IP),
							},
						},
					},
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
		},
	}
}

func ip(instance *unstructured.Unstructured) (string, error) {
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

func owner(instance *unstructured.Unstructured) (string, error) {
	if instance.GetOwnerReferences() == nil {
		return "", fmt.Errorf("sqlInstance %s has no ownerReference", instance.GetName())
	}

	if len(instance.GetOwnerReferences()) != 1 {
		return "", fmt.Errorf("sqlInstance %s has more than one ownerReference", instance.GetName())
	}

	o := instance.GetOwnerReferences()[0]
	if o.Kind != "Application" && o.Kind != "NaisJob" {
		return "", fmt.Errorf("sqlInstance %s has ownerReference of kind %s, expected Application or NaisJob", instance.GetName(), o.Kind)
	}
	return o.Name, nil
}
