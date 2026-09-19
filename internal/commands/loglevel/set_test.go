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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/loglevel"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Set", func() {
	It("set updates default DebuggingConfiguration instance", func() {

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		Expect(loglevel.UpdateDebuggingConfiguration(context.TODO(), libsveltosv1beta1.LogLevelDebug,
			string(libsveltosv1beta1.ComponentAddonManager))).To(Succeed())

		k8sAccess := utils.GetAccessInstance()

		currentDC, err := k8sAccess.GetDebuggingConfiguration(context.TODO())
		Expect(err).To(BeNil())
		Expect(currentDC).ToNot(BeNil())
		Expect(currentDC.Spec.Configuration).ToNot(BeNil())
		Expect(len(currentDC.Spec.Configuration)).To(Equal(1))
		Expect(currentDC.Spec.Configuration[0].Component).To(Equal(libsveltosv1beta1.ComponentAddonManager))
		Expect(currentDC.Spec.Configuration[0].LogLevel).To(Equal(libsveltosv1beta1.LogLevelDebug))

		Expect(loglevel.UpdateDebuggingConfiguration(context.TODO(), libsveltosv1beta1.LogLevelInfo,
			string(libsveltosv1beta1.ComponentAddonManager))).To(Succeed())
		currentDC, err = k8sAccess.GetDebuggingConfiguration(context.TODO())
		Expect(err).To(BeNil())
		Expect(currentDC).ToNot(BeNil())
		Expect(currentDC.Spec.Configuration).ToNot(BeNil())
		Expect(len(currentDC.Spec.Configuration)).To(Equal(1))
		Expect(currentDC.Spec.Configuration[0].Component).To(Equal(libsveltosv1beta1.ComponentAddonManager))
		Expect(currentDC.Spec.Configuration[0].LogLevel).To(Equal(libsveltosv1beta1.LogLevelInfo))
	})

	It("collectLogLevelConfigurationFromClient returns empty configuration when DebuggingConfiguration does not exist", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		configs, err := loglevel.CollectLogLevelConfigurationFromClient(context.TODO(), c)
		Expect(err).To(BeNil())
		Expect(configs).ToNot(BeNil())
		Expect(len(configs)).To(Equal(0))
	})

	It("collectLogLevelConfigurationFromClient returns existing configuration", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())

		// Create a DebuggingConfiguration with some initial settings
		dc := &libsveltosv1beta1.DebuggingConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: libsveltosv1beta1.DebuggingConfigurationSpec{
				Configuration: []libsveltosv1beta1.ComponentConfiguration{
					{
						Component: libsveltosv1beta1.ComponentClassifier,
						LogLevel:  libsveltosv1beta1.LogLevelDebug,
					},
					{
						Component: libsveltosv1beta1.ComponentAddonManager,
						LogLevel:  libsveltosv1beta1.LogLevelInfo,
					},
				},
			},
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dc).Build()

		configs, err := loglevel.CollectLogLevelConfigurationFromClient(context.TODO(), c)
		Expect(err).To(BeNil())
		Expect(configs).ToNot(BeNil())
		Expect(len(configs)).To(Equal(2))
	})

	It("updateLogLevelConfigurationWithClient creates new DebuggingConfiguration when it does not exist", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		spec := []libsveltosv1beta1.ComponentConfiguration{
			{
				Component: libsveltosv1beta1.ComponentClassifier,
				LogLevel:  libsveltosv1beta1.LogLevelDebug,
			},
		}

		err = loglevel.UpdateLogLevelConfigurationWithClient(context.TODO(), c, spec)
		Expect(err).To(BeNil())

		// Verify the configuration was created
		configs, err := loglevel.CollectLogLevelConfigurationFromClient(context.TODO(), c)
		Expect(err).To(BeNil())
		Expect(configs).ToNot(BeNil())
		Expect(len(configs)).To(Equal(1))
	})

	It("updateLogLevelConfigurationWithClient updates existing DebuggingConfiguration", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())

		// Create a DebuggingConfiguration with initial settings
		dc := &libsveltosv1beta1.DebuggingConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: "default",
			},
			Spec: libsveltosv1beta1.DebuggingConfigurationSpec{
				Configuration: []libsveltosv1beta1.ComponentConfiguration{
					{
						Component: libsveltosv1beta1.ComponentClassifier,
						LogLevel:  libsveltosv1beta1.LogLevelInfo,
					},
				},
			},
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dc).Build()

		// Update the configuration
		spec := []libsveltosv1beta1.ComponentConfiguration{
			{
				Component: libsveltosv1beta1.ComponentClassifier,
				LogLevel:  libsveltosv1beta1.LogLevelDebug,
			},
			{
				Component: libsveltosv1beta1.ComponentAddonManager,
				LogLevel:  libsveltosv1beta1.LogLevelVerbose,
			},
		}

		err = loglevel.UpdateLogLevelConfigurationWithClient(context.TODO(), c, spec)
		Expect(err).To(BeNil())

		// Verify the configuration was updated
		configs, err := loglevel.CollectLogLevelConfigurationFromClient(context.TODO(), c)
		Expect(err).To(BeNil())
		Expect(configs).ToNot(BeNil())
		Expect(len(configs)).To(Equal(2))
	})
})
