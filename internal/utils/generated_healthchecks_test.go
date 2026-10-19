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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("GeneratedHealthChecks", func() {
	// generatedHealthCheck returns the HealthCheck generated for the ClusterSummary identified by uid
	generatedHealthCheck := func(uid, summaryNamespace, summaryName string) *libsveltosv1beta1.HealthCheck {
		return &libsveltosv1beta1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hm-" + randomString(),
				Labels: map[string]string{
					configv1beta1.HealthMonitoringClusterSummaryUIDLabel: uid,
				},
				Annotations: map[string]string{
					configv1beta1.HealthMonitoringClusterSummaryNamespaceAnnotation: summaryNamespace,
					configv1beta1.HealthMonitoringClusterSummaryNameAnnotation:      summaryName,
				},
			},
		}
	}

	listGeneratedHealthChecks := func(uids []string, objects ...client.Object) map[types.NamespacedName]string {
		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		generatedChecks, err := k8sAccess.ListGeneratedHealthChecks(context.TODO(), uids,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		return generatedChecks
	}

	It("ListGeneratedHealthChecks maps each requested ClusterSummary to its generated HealthCheck", func() {
		summaryNamespace := randomString()
		summaryName := randomString()
		uid := randomString()

		generated := generatedHealthCheck(uid, summaryNamespace, summaryName)
		// Generated for a ClusterSummary which was not requested
		other := generatedHealthCheck(randomString(), summaryNamespace, randomString())
		// A HealthCheck created by a user is not a generated one
		userCreated := &libsveltosv1beta1.HealthCheck{ObjectMeta: metav1.ObjectMeta{Name: randomString()}}

		generatedChecks := listGeneratedHealthChecks([]string{uid}, generated, other, userCreated)
		Expect(generatedChecks).To(HaveLen(1))
		Expect(generatedChecks[types.NamespacedName{Namespace: summaryNamespace, Name: summaryName}]).
			To(Equal(generated.Name))
	})

	It("ListGeneratedHealthChecks does not list anything when no ClusterSummary is requested", func() {
		generated := generatedHealthCheck(randomString(), randomString(), randomString())

		Expect(listGeneratedHealthChecks(nil, generated)).To(BeEmpty())
	})

	// maxSelectorValues is 100 and above 20 batches all the generated HealthChecks are listed at once:
	// check both ways of listing return the requested HealthChecks, and only those.
	for _, count := range []int{250, 2100} {
		It(fmt.Sprintf("ListGeneratedHealthChecks returns only the requested HealthChecks when asking for %d", count), func() {
			summaryNamespace := randomString()

			uids := make([]string, 0, count)
			objects := make([]client.Object, 0, count+10)
			expected := map[types.NamespacedName]string{}
			for i := 0; i < count; i++ {
				uid := randomString()
				summaryName := fmt.Sprintf("summary-%d", i)
				healthCheck := generatedHealthCheck(uid, summaryNamespace, summaryName)
				uids = append(uids, uid)
				objects = append(objects, healthCheck)
				expected[types.NamespacedName{Namespace: summaryNamespace, Name: summaryName}] = healthCheck.Name
			}
			for i := 0; i < 10; i++ {
				objects = append(objects, generatedHealthCheck(randomString(), summaryNamespace, randomString()))
			}

			Expect(listGeneratedHealthChecks(uids, objects...)).To(Equal(expected))
		})
	}
})
