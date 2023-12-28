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
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Classifier", func() {
	It("ListClassifiers returns list of all classifiers", func() {
		initObjects := []client.Object{}

		for i := 0; i < 10; i++ {
			classifier := &libsveltosv1alpha1.Classifier{
				ObjectMeta: metav1.ObjectMeta{
					Name: randomString(),
				},
				Spec: libsveltosv1alpha1.ClassifierSpec{
					ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
						{Key: randomString(), Value: randomString()},
					},
					KubernetesVersionConstraints: []libsveltosv1alpha1.KubernetesVersionConstraint{
						{Version: randomString(), Comparison: string(libsveltosv1alpha1.ComparisonEqual)},
					},
					DeployedResourceConstraints: []libsveltosv1alpha1.DeployedResourceConstraint{
						{
							Group:   randomString(),
							Version: randomString(),
							Kind:    randomString(),
						},
					},
				},
			}
			initObjects = append(initObjects, classifier)
		}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		classifiers, err := k8sAccess.ListClassifiers(context.TODO(),
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(classifiers.Items)).To(Equal(len(initObjects)))
	})
})
