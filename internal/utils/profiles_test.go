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

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Profile", func() {
	It("ListProfiles returns list of all profiles", func() {
		initObjects := []client.Object{}

		for i := 0; i < 15; i++ {
			profile := &configv1beta1.Profile{
				ObjectMeta: metav1.ObjectMeta{
					Name: randomString(),
				},
				Spec: configv1beta1.Spec{
					ClusterSelector: libsveltosv1beta1.Selector{
						LabelSelector: metav1.LabelSelector{
							MatchLabels: map[string]string{"zone": "west"},
						},
					},
					SyncMode: configv1beta1.SyncModeContinuous,
				},
			}
			initObjects = append(initObjects, profile)
		}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		profiles, err := k8sAccess.ListProfiles(context.TODO(),
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(profiles.Items)).To(Equal(len(initObjects)))
	})
})
