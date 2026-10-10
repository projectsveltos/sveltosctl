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
package show_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("DriftHistory", func() {
	var namespace string
	var clusterName string
	var profileName string

	BeforeEach(func() {
		namespace = namePrefix + randomString()
		clusterName = randomString()
		profileName = randomString()
	})

	// generateDriftClusterSummary returns a ClusterSummary, owned by a ClusterProfile, for the
	// cluster identified by namespace/clusterName/clusterType, with the given feature summaries.
	generateDriftClusterSummary := func(namespace, clusterName, profileName string,
		clusterType libsveltosv1beta1.ClusterType,
		featureSummaries ...configv1beta1.FeatureSummary) *configv1beta1.ClusterSummary {

		return &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(clusterType),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: configv1beta1.GroupVersion.String(),
						Kind:       configv1beta1.ClusterProfileKind,
						Name:       profileName,
						UID:        "profile-uid",
					},
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: namespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
			},
			Status: configv1beta1.ClusterSummaryStatus{
				FeatureSummaries: featureSummaries,
			},
		}
	}

	driftedResource := func(group, kind, namespace, name string, detected time.Time) configv1beta1.DriftedResourceRef {
		return configv1beta1.DriftedResourceRef{
			Group:        group,
			Kind:         kind,
			Namespace:    namespace,
			Name:         name,
			DetectedTime: metav1.NewTime(detected),
		}
	}

	driftHistory := func(resources ...configv1beta1.DriftedResourceRef) *configv1beta1.DriftHistory {
		history := &configv1beta1.DriftHistory{Resources: resources}
		if len(resources) > 0 {
			history.LastDetectedTime = resources[0].DetectedTime
		}
		return history
	}

	// runDriftHistory runs show drift-history against the given objects and returns what it printed
	runDriftHistory := func(filter show.DriftHistoryFilter, objects ...client.Object) string {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayDriftHistory(context.TODO(), &filter,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())
		os.Stdout = old

		return buf.String()
	}

	// findLine returns the output line containing all the passed strings, or "" if there is none
	findLine := func(output string, contains ...string) string {
		for _, line := range strings.Split(output, "\n") {
			found := true
			for i := range contains {
				if !strings.Contains(line, contains[i]) {
					found = false
					break
				}
			}
			if found {
				return line
			}
		}
		return ""
	}

	It("show drift-history lists drifted resources, most recent first, with RFC3339 UTC times", func() {
		newer := time.Date(2026, 10, 10, 10, 56, 21, 0, time.UTC)
		older := time.Date(2026, 10, 10, 10, 28, 36, 0, time.UTC)

		releaseNamespace := "nginx"
		releaseName := "ingress"

		serviceAccount := driftedResource("", "ServiceAccount", releaseNamespace, "nginx-sa", newer)
		serviceAccount.HelmReleaseNamespace = releaseNamespace
		serviceAccount.HelmReleaseName = releaseName
		// Time given with a +02:00 offset must still be displayed in UTC
		deployment := driftedResource("apps", "Deployment", releaseNamespace, "nginx-deploy",
			older.In(time.FixedZone("CEST", 2*60*60)))
		deployment.HelmReleaseNamespace = releaseNamespace
		deployment.HelmReleaseName = releaseName

		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeSveltos,
			configv1beta1.FeatureSummary{
				FeatureID:    libsveltosv1beta1.FeatureHelm,
				DriftHistory: driftHistory(serviceAccount, deployment),
			})

		output := runDriftHistory(show.DriftHistoryFilter{}, clusterSummary)

		clusterInfo := namespace + "/" + clusterName
		serviceAccountLine := findLine(output, clusterInfo, "ClusterProfile/"+profileName, "Helm",
			"ServiceAccount", "nginx-sa", "2026-10-10T10:56:21Z", releaseNamespace+"/"+releaseName)
		Expect(serviceAccountLine).ToNot(BeEmpty(), output)
		deploymentLine := findLine(output, clusterInfo, "Deployment.apps", "nginx-deploy",
			"2026-10-10T10:28:36Z", releaseNamespace+"/"+releaseName)
		Expect(deploymentLine).ToNot(BeEmpty(), output)

		Expect(strings.Index(output, serviceAccountLine)).To(BeNumerically("<", strings.Index(output, deploymentLine)))
	})

	It("show drift-history displays a dash for the columns which do not apply", func() {
		namespaceless := driftedResource("rbac.authorization.k8s.io", "ClusterRole", "", "reader",
			time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC))

		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeCapi,
			configv1beta1.FeatureSummary{
				FeatureID:    libsveltosv1beta1.FeatureResources,
				DriftHistory: driftHistory(namespaceless),
			})

		output := runDriftHistory(show.DriftHistoryFilter{}, clusterSummary)

		line := findLine(output, "ClusterRole.rbac.authorization.k8s.io", "reader", "2026-10-10T09:00:00Z")
		Expect(line).ToNot(BeEmpty(), output)
		Expect(strings.Count(line, " - ")).To(Equal(2), line) // namespace and helm release
	})

	It("show drift-history displays a row when a drift was detected but resources were not reported", func() {
		detected := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeSveltos,
			configv1beta1.FeatureSummary{
				FeatureID: libsveltosv1beta1.FeatureKustomize,
				DriftHistory: &configv1beta1.DriftHistory{
					LastDetectedTime: metav1.NewTime(detected),
				},
			})

		output := runDriftHistory(show.DriftHistoryFilter{}, clusterSummary)

		Expect(findLine(output, "Kustomize", "(not reported)", "2026-10-10T08:00:00Z")).ToNot(BeEmpty(), output)
	})

	It("show drift-history mentions when the list of drifted resources is truncated", func() {
		history := driftHistory(driftedResource("", "ConfigMap", "default", "cm",
			time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)))
		history.Truncated = true
		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeSveltos,
			configv1beta1.FeatureSummary{FeatureID: libsveltosv1beta1.FeatureResources, DriftHistory: history})

		output := runDriftHistory(show.DriftHistoryFilter{}, clusterSummary)

		Expect(findLine(output, "Note:", namespace+"/"+clusterName, "Resources", "incomplete")).ToNot(BeEmpty(), output)
	})

	It("show drift-history ignores features without drift history", func() {
		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeSveltos,
			configv1beta1.FeatureSummary{FeatureID: libsveltosv1beta1.FeatureHelm},
			configv1beta1.FeatureSummary{
				FeatureID: libsveltosv1beta1.FeatureResources,
				DriftHistory: driftHistory(driftedResource("", "Secret", "default", "only-resources",
					time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC))),
			})

		output := runDriftHistory(show.DriftHistoryFilter{}, clusterSummary)

		Expect(findLine(output, "only-resources")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "Helm")).To(BeEmpty(), output)
	})

	It("show drift-history filters by namespace, cluster, cluster type, profile and feature", func() {
		detected := time.Date(2026, 10, 10, 8, 0, 0, 0, time.UTC)
		helmFeature := configv1beta1.FeatureSummary{
			FeatureID:    libsveltosv1beta1.FeatureHelm,
			DriftHistory: driftHistory(driftedResource("apps", "Deployment", "d", "helm-resource", detected)),
		}
		resourcesFeature := configv1beta1.FeatureSummary{
			FeatureID:    libsveltosv1beta1.FeatureResources,
			DriftHistory: driftHistory(driftedResource("", "ConfigMap", "d", "plain-resource", detected)),
		}

		clusterSummary := generateDriftClusterSummary(namespace, clusterName, profileName,
			libsveltosv1beta1.ClusterTypeSveltos, helmFeature, resourcesFeature)

		otherNamespace := namePrefix + randomString()
		otherClusterSummary := generateDriftClusterSummary(otherNamespace, randomString(), randomString(),
			libsveltosv1beta1.ClusterTypeCapi,
			configv1beta1.FeatureSummary{
				FeatureID:    libsveltosv1beta1.FeatureHelm,
				DriftHistory: driftHistory(driftedResource("", "Service", "d", "other-resource", detected)),
			})

		objects := []client.Object{clusterSummary, otherClusterSummary}

		output := runDriftHistory(show.DriftHistoryFilter{}, objects...)
		Expect(findLine(output, "helm-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "plain-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "other-resource")).ToNot(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{Namespace: namespace}, objects...)
		Expect(findLine(output, "helm-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "other-resource")).To(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{Cluster: clusterName}, objects...)
		Expect(findLine(output, "helm-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "other-resource")).To(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{ClusterType: string(libsveltosv1beta1.ClusterTypeCapi)},
			objects...)
		Expect(findLine(output, "other-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "helm-resource")).To(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{Profile: profileName}, objects...)
		Expect(findLine(output, "helm-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "other-resource")).To(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{Feature: "helm"}, objects...)
		Expect(findLine(output, "helm-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "other-resource")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "plain-resource")).To(BeEmpty(), output)

		output = runDriftHistory(show.DriftHistoryFilter{Profile: randomString()}, objects...)
		Expect(findLine(output, "-resource")).To(BeEmpty(), output)
	})
})
