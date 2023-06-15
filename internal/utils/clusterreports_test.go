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

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("ClusterReport", func() {
	//nolint: dupl // exception for a test
	It("ListClusterReports returns list of all clusterReports", func() {
		initObjects := []client.Object{}

		for i := 0; i < 5; i++ {
			clusterReport := &configv1alpha1.ClusterReport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      randomString(),
					Namespace: randomString(),
				},
			}
			initObjects = append(initObjects, clusterReport)
		}

		clusterReport := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}
		initObjects = append(initObjects, clusterReport)

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		clusterReports, err := k8sAccess.ListClusterReports(context.TODO(), "", klogr.New())
		Expect(err).To(BeNil())
		Expect(len(clusterReports.Items)).To(Equal(len(initObjects)))

		clusterReports, err = k8sAccess.ListClusterReports(context.TODO(),
			clusterReport.Namespace, klogr.New())
		Expect(err).To(BeNil())
		Expect(len(clusterReports.Items)).To(Equal(1))
	})

	It("SortClusterReports sorts clusterReports by Cluster Namespace/Name", func() {
		clusterReports := []configv1alpha1.ClusterReport{}

		firstNamespace := "namespace-a"
		secondNamespace := "namespace-b"

		clusterReport := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterName:      randomString(),
				ClusterNamespace: secondNamespace,
			},
		}
		clusterReports = append(clusterReports, *clusterReport)

		clusterReport = &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterName:      randomString(),
				ClusterNamespace: firstNamespace,
			},
		}
		clusterReports = append(clusterReports, *clusterReport)

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		clusterReports = k8sAccess.SortClusterReports(clusterReports)
		Expect(len(clusterReports)).To(Equal(2))
		Expect(clusterReports[0].Spec.ClusterNamespace).To(Equal(firstNamespace))
		Expect(clusterReports[1].Spec.ClusterNamespace).To(Equal(secondNamespace))
	})
})
