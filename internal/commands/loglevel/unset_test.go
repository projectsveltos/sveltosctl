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

package loglevel_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/loglevel"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Unset", func() {
	It("unset removes log level settings in managed cluster", func() {
		dc := getDebuggingConfiguration()
		dc.Spec.Configuration = []libsveltosv1alpha1.ComponentConfiguration{
			{Component: libsveltosv1alpha1.ComponentClassifier, LogLevel: libsveltosv1alpha1.LogLevelInfo},
			{Component: libsveltosv1alpha1.ComponentAddonManager, LogLevel: libsveltosv1alpha1.LogLevelInfo},
		}

		initObjects := []client.Object{dc}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(loglevel.UnsetDebuggingConfiguration(context.TODO(), string(libsveltosv1alpha1.ComponentClassifier), "", "", "")).To(Succeed())

		k8sAccess := utils.GetAccessInstance()

		currentDC, err := k8sAccess.GetDebuggingConfiguration(context.TODO(), "", "", "")
		Expect(err).To(BeNil())
		Expect(currentDC).ToNot(BeNil())
		Expect(currentDC.Spec.Configuration).ToNot(BeNil())
		Expect(len(currentDC.Spec.Configuration)).To(Equal(1))
		Expect(currentDC.Spec.Configuration[0].Component).To(Equal(libsveltosv1alpha1.ComponentAddonManager))
		Expect(currentDC.Spec.Configuration[0].LogLevel).To(Equal(libsveltosv1alpha1.LogLevelInfo))
	})
})

