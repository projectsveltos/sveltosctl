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

var _ = Describe("HealthCheck", func() {
	It("ListHealthChecks returns list of all HealthChecks", func() {
		initObjects := []client.Object{}

		for i := 0; i < 10; i++ {
			hc := &libsveltosv1beta1.HealthCheck{
				ObjectMeta: metav1.ObjectMeta{
					Name: randomString(),
				},
				Spec: libsveltosv1beta1.HealthCheckSpec{
					ResourceSelectors: []libsveltosv1beta1.ResourceSelector{
						{
							Group:   randomString(),
							Version: randomString(),
							Kind:    randomString(),
						},
					},
				},
			}
			initObjects = append(initObjects, hc)
		}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		healthChecks, err := k8sAccess.ListHealthChecks(context.TODO(),
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(healthChecks.Items)).To(Equal(len(initObjects)))
	})
})
