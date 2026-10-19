/*
Copyright 2026. projectsveltos.io. All rights reserved.

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
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("ClusterHealthCheckReports", func() {
	// healthCheckReport returns a HealthCheckReport labeled the way the report of a cluster is
	healthCheckReport := func(namespace, clusterName string,
		clusterType libsveltosv1beta1.ClusterType) *libsveltosv1beta1.HealthCheckReport {

		healthCheckName := randomString()
		return &libsveltosv1beta1.HealthCheckReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				Labels:    libsveltosv1beta1.GetHealthCheckReportLabels(healthCheckName, clusterName, &clusterType),
			},
			Spec: libsveltosv1beta1.HealthCheckReportSpec{
				ClusterNamespace: namespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				HealthCheckName:  healthCheckName,
			},
		}
	}

	It("ListClusterHealthCheckReports returns only the reports of the selected clusters", func() {
		namespace := randomString()
		clusterName := randomString()

		selected := healthCheckReport(namespace, clusterName, libsveltosv1beta1.ClusterTypeSveltos)
		otherType := healthCheckReport(namespace, clusterName, libsveltosv1beta1.ClusterTypeCapi)
		otherCluster := healthCheckReport(namespace, randomString(), libsveltosv1beta1.ClusterTypeSveltos)
		otherNamespace := healthCheckReport(randomString(), clusterName, libsveltosv1beta1.ClusterTypeSveltos)
		objects := []client.Object{selected, otherType, otherCluster, otherNamespace}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
		k8sAccess := utils.GetK8sAccess(scheme, c)
		logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))

		reports, err := k8sAccess.ListClusterHealthCheckReports(context.TODO(), "", "", "", logger)
		Expect(err).To(BeNil())
		Expect(reports.Items).To(HaveLen(len(objects)))

		reports, err = k8sAccess.ListClusterHealthCheckReports(context.TODO(), namespace, clusterName, "Sveltos", logger)
		Expect(err).To(BeNil())
		Expect(reports.Items).To(HaveLen(1))
		Expect(reports.Items[0].Name).To(Equal(selected.Name))

		reports, err = k8sAccess.ListClusterHealthCheckReports(context.TODO(), "", clusterName, "", logger)
		Expect(err).To(BeNil())
		Expect(reports.Items).To(HaveLen(3))

		reports, err = k8sAccess.ListClusterHealthCheckReports(context.TODO(), namespace, "", "Capi", logger)
		Expect(err).To(BeNil())
		Expect(reports.Items).To(HaveLen(1))
		Expect(reports.Items[0].Name).To(Equal(otherType.Name))
	})
})
