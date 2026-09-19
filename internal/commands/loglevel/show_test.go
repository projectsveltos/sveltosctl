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
	"bytes"
	"context"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/loglevel"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Show", func() {
	It("show displays current log level settings", func() {
		dc := getDebuggingConfiguration()
		dc.Spec.Configuration = []libsveltosv1beta1.ComponentConfiguration{
			{Component: libsveltosv1beta1.ComponentClassifier, LogLevel: libsveltosv1beta1.LogLevelDebug},
		}

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{dc}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = loglevel.ShowLogSettings(context.TODO())
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			   +------------------+---------------+
			   |      COMPONENT   | VERBOSIRY     |
			   +------------------+---------------+
			   | Classifier       | LogLevelDebug |
			   +------------------+---------------+
		*/

		lines := strings.Split(buf.String(), "\n")
		found := false
		for i := range lines {
			if strings.Contains(lines[i], string(libsveltosv1beta1.ComponentClassifier)) &&
				strings.Contains(lines[i], string(libsveltosv1beta1.LogLevelDebug)) {
				found = true
				break
			}
		}

		Expect(found).To(BeTrue())
		os.Stdout = old
	})

	It("showLogSettingsInManaged displays log level settings from managed cluster", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())

		// Create a DebuggingConfiguration for the managed cluster
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

		managedClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dc).Build()

		// Test the helper function directly with the managed client
		componentConfiguration, err := loglevel.CollectLogLevelConfigurationFromClient(context.TODO(), managedClient)
		Expect(err).To(BeNil())
		Expect(componentConfiguration).ToNot(BeNil())
		Expect(len(componentConfiguration)).To(Equal(1))
	})
})
