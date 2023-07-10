/*
Copyright 2023. projectsveltos.io. All rights reserved.

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

package show_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Resources", func() {
	It("show resources displays resources from various managed clusters", func() {
		message := "All replicas 1 are healthy"

		hcr := &libsveltosv1alpha1.HealthCheckReport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: libsveltosv1alpha1.HealthCheckReportSpec{
				ClusterNamespace: randomString(),
				ClusterName:      randomString(),
				ClusterType:      libsveltosv1alpha1.ClusterTypeSveltos,
				ResourceStatuses: []libsveltosv1alpha1.ResourceStatus{
					{
						Resource: nil,
						ObjectRef: corev1.ObjectReference{
							Kind:       "Deployment",
							APIVersion: appsv1.SchemeGroupVersion.String(),
							Namespace:  randomString(),
							Name:       randomString(),
						},
						Message:      message,
						HealthStatus: libsveltosv1alpha1.HealthStatusHealthy,
					},
					{
						Resource: nil,
						ObjectRef: corev1.ObjectReference{
							Kind:       "Service",
							APIVersion: corev1.SchemeGroupVersion.String(),
							Namespace:  randomString(),
							Name:       randomString(),
						},
						Message:      message,
						HealthStatus: libsveltosv1alpha1.HealthStatusHealthy,
					},
				},
			},
		}

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{hcr}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayResources(context.TODO(), "", "", "", "", "", false, klogr.New())
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
			|               CLUSTER               |           GVK            |   NAMESPACE    |          NAME           |          MESSAGE           |
			+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
			| default/sveltos-management-workload | apps/v1, Kind=Deployment | kube-system    | calico-kube-controllers | All replicas 1 are healthy |
			|                                     |                          | kube-system    | coredns                 | All replicas 2 are healthy |
			|                                     |                          | projectsveltos | sveltos-agent-manager   | All replicas 1 are healthy |
			+-------------------------------------+--------------------------+----------------+-------------------------+----------------------------+
		*/

		lines := strings.Split(buf.String(), "\n")
		for i := range hcr.Spec.ResourceStatuses {
			verifyDisplayedResources(lines, &hcr.Spec.ResourceStatuses[i].ObjectRef,
				hcr.Spec.ResourceStatuses[i].Message)
		}
		os.Stdout = old
	})
})

func verifyDisplayedResources(lines []string, resource *corev1.ObjectReference, message string) {
	found := false
	for i := range lines {
		if strings.Contains(lines[i], resource.Kind) &&
			strings.Contains(lines[i], resource.APIVersion) &&
			strings.Contains(lines[i], resource.Namespace) &&
			strings.Contains(lines[i], resource.Name) &&
			strings.Contains(lines[i], message) {

			found = true
			break
		}
	}
	if found != true {
		By(fmt.Sprintf("Failed to verify resource %s:%s/%s", resource.Kind, resource.Namespace, resource.Name))
		By(fmt.Sprintf("Results: %v", lines))
	}
	Expect(found).To(BeTrue())
}
