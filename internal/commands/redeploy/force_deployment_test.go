/*
Copyright 2025. projectsveltos.io. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package redeploy_test

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	"github.com/projectsveltos/addon-controller/lib/clusterops"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/redeploy"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// newProfileOwnerReference returns an OwnerReference like the one addon-controller sets on
// every ClusterSummary it creates, the only part getClusterSummariesInOrder reads off it.
func newProfileOwnerReference(profileName string) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion: configv1beta1.GroupVersion.String(),
		Kind:       configv1beta1.ClusterProfileKind,
		Name:       profileName,
		UID:        types.UID(randomString()),
	}
}

var _ = Describe("Redeploy cluster", func() {
	var logger logr.Logger

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig())
	})

	It("resetClusterSummaryInstance resets ClusterSummary Status", func() {
		clusterNamespace := randomString()
		clusterName := randomString()
		clusterType := libsveltosv1beta1.ClusterTypeSveltos

		clusterSummary := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: clusterNamespace,
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Status: configv1beta1.ClusterSummaryStatus{
				FeatureSummaries: []configv1beta1.FeatureSummary{
					{
						FeatureID: libsveltosv1beta1.FeatureResources,
						Status:    libsveltosv1beta1.FeatureStatusProvisioned,
						Hash:      []byte(randomString()),
					},
					{
						FeatureID: libsveltosv1beta1.FeatureHelm,
						Status:    libsveltosv1beta1.FeatureStatusProvisioned,
						Hash:      []byte(randomString()),
					},
				},
			},
		}

		clusterSummary2 := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: clusterNamespace,
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName + randomString(),
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Status: configv1beta1.ClusterSummaryStatus{
				FeatureSummaries: []configv1beta1.FeatureSummary{
					{
						FeatureID: libsveltosv1beta1.FeatureResources,
						Status:    libsveltosv1beta1.FeatureStatusProvisioned,
						Hash:      []byte(randomString()),
					},
					{
						FeatureID: libsveltosv1beta1.FeatureHelm,
						Status:    libsveltosv1beta1.FeatureStatusProvisioned,
						Hash:      []byte(randomString()),
					},
				},
			},
		}

		initObjects := []client.Object{clusterSummary, clusterSummary2}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(initObjects...).
			WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(redeploy.ResetClusterSummaryInstance(context.TODO(), clusterNamespace, clusterName,
			&clusterType, logger)).To(Succeed())

		currentClusterSummary := &configv1beta1.ClusterSummary{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterSummary.Name},
			currentClusterSummary)).To(Succeed())
		Expect(len(currentClusterSummary.Status.FeatureSummaries)).To(Equal(0))

		// ClusterSummary instances for other clusters are not reset
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterSummary2.Name},
			currentClusterSummary)).To(Succeed())
		Expect(len(currentClusterSummary.Status.FeatureSummaries)).To(Equal(2))
	})

	It("getClusterSummariesInOrder returns ClusterSummary in right order based on dependsOn", func() {
		clusterNamespace := randomString()
		clusterName := randomString()
		clusterType := libsveltosv1beta1.ClusterTypeSveltos

		// DependsOn/OwnerReferences name (Cluster)Profiles, never the derived ClusterSummary
		// name directly, so the fixtures below mirror what addon-controller actually creates:
		// a bare profile name in DependsOn, and the real name/kind on OwnerReferences.
		profile1Name := randomString()
		profile2Name := randomString()
		profile3Name := randomString()
		profile4Name := randomString()

		clusterSummary1 := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:            clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind, profile1Name, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(profile1Name)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
			},
		}

		clusterSummary2 := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:            clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind, profile2Name, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(profile2Name)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				ClusterProfileSpec: configv1beta1.Spec{
					DependsOn: []string{profile1Name},
				},
			},
		}

		clusterSummary3 := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:            clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind, profile3Name, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(profile3Name)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				ClusterProfileSpec: configv1beta1.Spec{
					DependsOn: []string{profile2Name},
				},
			},
		}

		clusterSummary4 := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name:            clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind, profile4Name, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(profile4Name)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				ClusterProfileSpec: configv1beta1.Spec{
					DependsOn: []string{profile2Name, profile3Name},
				},
			},
		}

		initObjects := []client.Object{clusterSummary3, clusterSummary2, clusterSummary1, clusterSummary4}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(initObjects...).
			WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		resetOrder, csMap, err := redeploy.GetClusterSummariesInOrder(context.TODO(), c,
			clusterNamespace, clusterName, &clusterType)
		Expect(err).To(BeNil())

		Expect(len(csMap)).To(Equal(4))
		Expect(len(resetOrder)).To(Equal(4))
		Expect(resetOrder[0]).To(Equal(clusterSummary1.Name))
		Expect(resetOrder[1]).To(Equal(clusterSummary2.Name))
		Expect(resetOrder[2]).To(Equal(clusterSummary3.Name))
		Expect(resetOrder[3]).To(Equal(clusterSummary4.Name))
	})

	It("getClusterSummariesInOrder resets a TransitionFrom successor before its predecessor", func() {
		clusterNamespace := randomString()
		clusterName := randomString()
		clusterType := libsveltosv1beta1.ClusterTypeSveltos

		predecessorProfileName := randomString()
		successorProfileName := randomString()

		// Predecessor: no relationships of its own, matches e.g. sbom-scanner-v1.
		predecessorSummary := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind,
					predecessorProfileName, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(predecessorProfileName)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
			},
		}

		// Successor: declares it replaces the predecessor, e.g. sbom-scanner-v2.
		successorSummary := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterops.GetClusterSummaryName(configv1beta1.ClusterProfileKind,
					successorProfileName, clusterName, true),
				Namespace:       clusterNamespace,
				OwnerReferences: []metav1.OwnerReference{newProfileOwnerReference(successorProfileName)},
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				ClusterProfileSpec: configv1beta1.Spec{
					TransitionFrom: []string{predecessorProfileName},
				},
			},
		}

		initObjects := []client.Object{predecessorSummary, successorSummary}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(initObjects...).
			WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		resetOrder, csMap, err := redeploy.GetClusterSummariesInOrder(context.TODO(), c,
			clusterNamespace, clusterName, &clusterType)
		Expect(err).To(BeNil())

		Expect(len(csMap)).To(Equal(2))
		Expect(len(resetOrder)).To(Equal(2))
		// The predecessor's resumed teardown depends on the successor being reprovisioned,
		// so the successor must be reset first.
		Expect(resetOrder[0]).To(Equal(successorSummary.Name))
		Expect(resetOrder[1]).To(Equal(predecessorSummary.Name))
	})
})
