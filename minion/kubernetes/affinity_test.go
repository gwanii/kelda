package kubernetes

import (
	"testing"

	"github.com/kelda/kelda/db"
	"github.com/kelda/kelda/minion/kubernetes/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestToAffinities(t *testing.T) {
	t.Parallel()

	targetContainerA := "targetContainerA"
	targetContainerB := "targetContainerB"
	placements := []db.Placement{
		{TargetContainer: targetContainerA, Exclusive: true,
			OtherContainer: "other", Region: "us-west-1"},
		{TargetContainer: targetContainerA, FloatingIP: "8.8.8.8"},
		{TargetContainer: targetContainerB, Provider: "Amazon", Size: "m3.medium"},
	}

	affinityMap := toAffinities(placements)
	assert.Len(t, affinityMap, 2)
	assert.Equal(t, &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							hostnameKey: "other",
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      regionKey,
								Operator: corev1.NodeSelectorOpNotIn,
								Values:   []string{"us-west-1"},
							},
							{
								Key:      floatingIPKey,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"8.8.8.8"},
							},
						},
					},
				},
			},
		},
	}, affinityMap[targetContainerA])
	assert.Equal(t, &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      providerKey,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"Amazon"},
							},
							{
								Key:      sizeKey,
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"m3.medium"},
							},
						},
					},
				},
			},
		},
	}, affinityMap[targetContainerB])
}

func TestUpdateNodeLabels(t *testing.T) {
	t.Parallel()
	nodesClient := &mocks.NodeInterface{}

	// Test that we don't update anything when List fails.
	nodesClient.On("List", mock.Anything).Return(nil, assert.AnError).Once()
	updateNodeLabels(nil, nodesClient)
	nodesClient.AssertExpectations(t)

	// Test setting labels.
	minionA := db.Minion{
		PrivateIP: "8.8.8.8", Provider: "Amazon", FloatingIP: "floatingIP",
	}
	minionB := db.Minion{PrivateIP: "9.9.9.9", Provider: "Google"}
	nodes := []corev1.Node{
		// Node without an address. Even though we can't update the labels for
		// that node, we should update the others.
		{Status: corev1.NodeStatus{Addresses: nil}},

		// Node with an address that we don't have any information about. Even
		// though we can't update the labels for that node, we should update
		// the others.
		{Status: privateIPAddress("noInfo")},

		// The nodes that we will be updating.
		{Status: privateIPAddress(minionA.PrivateIP)},
		{Status: privateIPAddress(minionB.PrivateIP)},
	}
	nodeToUpdateA := corev1.Node{
		Status: privateIPAddress(minionA.PrivateIP),
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				providerKey:   minionA.Provider,
				floatingIPKey: minionA.FloatingIP,
				regionKey:     "",
				sizeKey:       "",
			},
		},
	}
	nodeToUpdateB := corev1.Node{
		Status: privateIPAddress(minionB.PrivateIP),
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				providerKey:   minionB.Provider,
				floatingIPKey: "",
				regionKey:     "",
				sizeKey:       "",
			},
		},
	}
	nodesClient.On("List", mock.Anything).Return(
		&corev1.NodeList{Items: nodes}, nil).Once()
	nodesClient.On("Update", &nodeToUpdateA).Return(nil, nil).Once()
	nodesClient.On("Update", &nodeToUpdateB).Return(nil, nil).Once()
	updateNodeLabels([]db.Minion{minionA, minionB}, nodesClient)
	nodesClient.AssertExpectations(t)

	// If the node labels are already correct, we shouldn't do anything.
	nodesClient.On("List", mock.Anything).Return(&corev1.NodeList{
		Items: []corev1.Node{nodeToUpdateA, nodeToUpdateB},
	}, nil).Once()
	updateNodeLabels([]db.Minion{minionA, minionB}, nodesClient)
	nodesClient.AssertExpectations(t)

	// Test that if the FloatingIP changes we update it.
	nodesClient.On("List", mock.Anything).Return(&corev1.NodeList{
		Items: []corev1.Node{nodeToUpdateA, nodeToUpdateB},
	}, nil).Once()

	minionA.FloatingIP = "changed"
	newNodeToUpdate := copyNode(nodeToUpdateA)
	newNodeToUpdate.Labels[floatingIPKey] = "changed"
	nodesClient.On("Update", &newNodeToUpdate).Return(nil, nil).Once()

	updateNodeLabels([]db.Minion{minionA, minionB}, nodesClient)
	nodesClient.AssertExpectations(t)
}

func privateIPAddress(ip string) corev1.NodeStatus {
	return corev1.NodeStatus{
		Addresses: []corev1.NodeAddress{
			{
				Type:    corev1.NodeInternalIP,
				Address: ip,
			},
		},
	}
}

func copyNode(node corev1.Node) (copy corev1.Node) {
	labelsCopy := map[string]string{}
	for k, v := range node.Labels {
		labelsCopy[k] = v
	}
	copy.Labels = labelsCopy
	copy.Status = node.Status
	return copy
}
