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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("AddOnss", func() {
	var clusterConfiguration *configv1alpha1.ClusterConfiguration
	var ns *corev1.Namespace

	BeforeEach(func() {
		namespace := namePrefix + randomString()

		clusterConfiguration = &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
			},
		}

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	})

	It("show addons displays deployed helm charts", func() {
		clusterProfileName1 := randomString()
		charts1 := []configv1alpha1.Chart{
			*generateChart(), *generateChart(),
		}
		clusterConfiguration = addDeployedHelmCharts(clusterConfiguration, clusterProfileName1, charts1)

		clusterProfileName2 := randomString()
		charts2 := []configv1alpha1.Chart{
			*generateChart(), *generateChart(), *generateChart(),
		}
		clusterConfiguration = addDeployedHelmCharts(clusterConfiguration, clusterProfileName2, charts2)

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{ns, clusterConfiguration}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayAddOns(context.TODO(), "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterConfiguration.Name)

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			+-------------------------------------+---------------+-----------+----------------+---------+--------------------+------------------+
			|               CLUSTER               | RESOURCE TYPE | NAMESPACE |      NAME      | VERSION |             TIME   | CLUSTER PROFILE  |
			+-------------------------------------+---------------+-----------+----------------+---------+--------------------+------------------+
			| default/sveltos-management-workload | helm chart    | kyverno   | kyverno-latest | v2.5.0  | 2022-09-30 11:48:45| active           |
			+-------------------------------------+---------------+-----------+----------------+---------+--------------------+------------------+
		*/

		lines := strings.Split(buf.String(), "\n")
		verifyCharts(lines, clusterInfo, clusterProfileName1, charts1)
		verifyCharts(lines, clusterInfo, clusterProfileName2, charts2)

		os.Stdout = old
	})

	It("show addonss display deployed resources", func() {
		clusterProfileName1 := randomString()
		resource1 := []configv1alpha1.Resource{
			*generateResource(), *generateResource(), *generateResource(),
		}
		clusterConfiguration = addDeployedResources(clusterConfiguration, clusterProfileName1, resource1)

		clusterProfileName2 := randomString()
		resource2 := []configv1alpha1.Resource{
			*generateResource(), *generateResource(), *generateResource(),
		}
		clusterConfiguration = addDeployedResources(clusterConfiguration, clusterProfileName2, resource2)

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{ns, clusterConfiguration}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayAddOns(context.TODO(), "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterConfiguration.Name)

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			+-------------------------------------+---------------+-----------+----------------+---------+---------------------+------------------+
			|               CLUSTER               | RESOURCE TYPE | NAMESPACE |      NAME      | VERSION |             TIME    | CLUSTER PROFILES |
			+-------------------------------------+---------------+-----------+----------------+---------+---------------------+------------------+
			| default/sveltos-management-workload | :Pod          | default   | nginx          | N/A     | 2022-09-30 13:41:05 | nginx-group      |
			+-------------------------------------+---------------+-----------+----------------+---------+---------------------+------------------+
		*/

		lines := strings.Split(buf.String(), "\n")
		verifyResources(lines, clusterInfo, clusterProfileName1, resource1)
		verifyResources(lines, clusterInfo, clusterProfileName2, resource2)

		os.Stdout = old
	})
})

func verifyCharts(lines []string, clusterInfo, clusterProfileName string,
	charts []configv1alpha1.Chart) {

	for i := range charts {
		verifyChart(lines, clusterInfo, clusterProfileName, &charts[i])
	}
}

func verifyChart(lines []string, clusterInfo, clusterProfileName string,
	chart *configv1alpha1.Chart) {

	found := false
	for i := range lines {
		if strings.Contains(lines[i], clusterInfo) &&
			strings.Contains(lines[i], clusterProfileName) &&
			strings.Contains(lines[i], chart.Namespace) &&
			strings.Contains(lines[i], chart.ReleaseName) {

			found = true
			break
		}
	}
	if found != true {
		By(fmt.Sprintf("Failed to verify chart %s/%s", chart.Namespace, chart.ReleaseName))
		By(fmt.Sprintf("Results: %v", lines))
	}
	Expect(found).To(BeTrue())
}

func verifyResources(lines []string, clusterInfo, clusterProfileName string,
	resources []configv1alpha1.Resource) {

	for i := range resources {
		verifyResource(lines, clusterInfo, clusterProfileName, &resources[i])
	}
}

func verifyResource(lines []string, clusterInfo, clusterProfileName string,
	resource *configv1alpha1.Resource) {

	found := false
	for i := range lines {
		if strings.Contains(lines[i], clusterInfo) &&
			strings.Contains(lines[i], clusterProfileName) &&
			strings.Contains(lines[i], resource.Namespace) &&
			strings.Contains(lines[i], resource.Name) &&
			strings.Contains(lines[i], resource.Group) &&
			strings.Contains(lines[i], resource.Kind) {

			found = true
			break
		}
	}
	if found != true {
		By(fmt.Sprintf("Failed to verify resource %s/%s", resource.Namespace, resource.Name))
		By(fmt.Sprintf("Results: %v", lines))
	}
	Expect(found).To(BeTrue())
}
