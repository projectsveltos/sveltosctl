/*
Copyright 2025. projectsveltos.io. All rights reserved.

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

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("HelmUpdates", func() {
	var namespace string
	var clusterName string
	var ns *corev1.Namespace

	BeforeEach(func() {
		namespace = namePrefix + randomString()
		clusterName = randomString()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	})

	// generateClusterConfiguration returns a ClusterConfiguration labeled so it, and any
	// ClusterSummary sharing the same labels/namespace, are considered to belong to the
	// same cluster.
	generateClusterConfiguration := func(namespace, clusterName string,
		clusterType libsveltosv1beta1.ClusterType) *configv1beta1.ClusterConfiguration {

		return &configv1beta1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
		}
	}

	// generateClusterSummary returns a ClusterSummary reporting outdated-version info for
	// the given chart, labeled to match the cluster identified by namespace/clusterName/clusterType.
	generateClusterSummary := func(namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType,
		chart *configv1beta1.Chart, latestVersion, latestPatchVersion string) *configv1beta1.ClusterSummary {

		return &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: namespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
			},
			Status: configv1beta1.ClusterSummaryStatus{
				HelmReleaseSummaries: []configv1beta1.HelmChartSummary{
					{
						ReleaseName:        chart.ReleaseName,
						ReleaseNamespace:   chart.Namespace,
						Status:             configv1beta1.HelmChartStatusManaging,
						LatestVersion:      strPtr(latestVersion),
						LatestPatchVersion: strPtr(latestPatchVersion),
					},
				},
			},
		}
	}

	It("show helm-updates lists only releases with a newer version or patch available", func() {
		clusterConfiguration := generateClusterConfiguration(namespace, clusterName, libsveltosv1beta1.ClusterTypeCapi)

		outdatedChart := generateChart()
		clusterConfiguration = addDeployedHelmCharts(clusterConfiguration, randomString(),
			[]configv1beta1.Chart{*outdatedChart})
		outdatedClusterSummary := generateClusterSummary(namespace, clusterName, libsveltosv1beta1.ClusterTypeCapi,
			outdatedChart, "v2.0.0", "")

		upToDateChart := generateChart()
		clusterConfiguration = addDeployedHelmCharts(clusterConfiguration, randomString(),
			[]configv1beta1.Chart{*upToDateChart})

		initObjects := []client.Object{ns, clusterConfiguration, outdatedClusterSummary}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayHelmUpdates(context.TODO(), "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())
		os.Stdout = old

		lines := strings.Split(buf.String(), "\n")

		found := false
		for i := range lines {
			if strings.Contains(lines[i], outdatedChart.Namespace) &&
				strings.Contains(lines[i], outdatedChart.ReleaseName) &&
				strings.Contains(lines[i], outdatedChart.ChartVersion) &&
				strings.Contains(lines[i], "v2.0.0") {

				found = true
				break
			}
		}
		Expect(found).To(BeTrue())

		for i := range lines {
			Expect(strings.Contains(lines[i], upToDateChart.ReleaseName)).To(BeFalse())
		}
	})

	It("show helm-updates filters by --cluster and --cluster-type", func() {
		clusterConfiguration := generateClusterConfiguration(namespace, clusterName, libsveltosv1beta1.ClusterTypeCapi)
		chart := generateChart()
		clusterConfiguration = addDeployedHelmCharts(clusterConfiguration, randomString(),
			[]configv1beta1.Chart{*chart})
		clusterSummary := generateClusterSummary(namespace, clusterName, libsveltosv1beta1.ClusterTypeCapi,
			chart, "v3.0.0", "")

		initObjects := []client.Object{ns, clusterConfiguration, clusterSummary}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		verifyOutputContainsRelease := func(clusterFilter, clusterTypeFilter string, expectFound bool) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err = show.DisplayHelmUpdates(context.TODO(), namespace, clusterFilter, clusterTypeFilter,
				textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
			Expect(err).To(BeNil())

			w.Close()
			var buf bytes.Buffer
			_, err = io.Copy(&buf, r)
			Expect(err).To(BeNil())
			os.Stdout = old

			found := strings.Contains(buf.String(), chart.ReleaseName)
			Expect(found).To(Equal(expectFound))
		}

		verifyOutputContainsRelease(clusterName, string(libsveltosv1beta1.ClusterTypeCapi), true)
		verifyOutputContainsRelease(randomString(), string(libsveltosv1beta1.ClusterTypeCapi), false)
		verifyOutputContainsRelease(clusterName, string(libsveltosv1beta1.ClusterTypeSveltos), false)
	})
})

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
