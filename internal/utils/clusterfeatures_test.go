/*
Copyright 2022. projectsveltos.io. All rights reserved.

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

package utils_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("ClusterFeature", func() {
	It("ListClusterFeatures returns list of all clusterFeatures", func() {

		initObjects := []client.Object{}

		for i := 0; i < 10; i++ {
			clusterFeature := &configv1alpha1.ClusterFeature{
				ObjectMeta: metav1.ObjectMeta{
					Name: randomString(),
				},
				Spec: configv1alpha1.ClusterFeatureSpec{
					ClusterSelector: configv1alpha1.Selector("zone:west"),
					SyncMode:        configv1alpha1.SyncModeContinuous,
				},
			}
			initObjects = append(initObjects, clusterFeature)
		}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		clusterFeatures, err := k8sAccess.ListClusterFeatures(context.TODO(), klogr.New())
		Expect(err).To(BeNil())
		Expect(len(clusterFeatures.Items)).To(Equal(len(initObjects)))
	})
})
