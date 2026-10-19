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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("UnhealthyResources", func() {
	var namespace string
	var clusterName string
	var profileName string

	BeforeEach(func() {
		namespace = namePrefix + randomString()
		clusterName = randomString()
		profileName = randomString()
	})

	// generateHealthClusterSummary returns a ClusterSummary, owned by a ClusterProfile, for the
	// Sveltos cluster identified by namespace/clusterName.
	generateHealthClusterSummary := func(namespace, clusterName, profileName string,
		monitoringEnabled bool) *configv1beta1.ClusterSummary {

		clusterSummary := &configv1beta1.ClusterSummary{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				UID:       types.UID(randomString()),
				Labels: map[string]string{
					configv1beta1.ClusterNameLabel: clusterName,
					configv1beta1.ClusterTypeLabel: string(libsveltosv1beta1.ClusterTypeSveltos),
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: configv1beta1.GroupVersion.String(),
						Kind:       configv1beta1.ClusterProfileKind,
						Name:       profileName,
						UID:        types.UID(randomString()),
					},
				},
			},
			Spec: configv1beta1.ClusterSummarySpec{
				ClusterNamespace: namespace,
				ClusterName:      clusterName,
				ClusterType:      libsveltosv1beta1.ClusterTypeSveltos,
			},
		}
		clusterSummary.Spec.ClusterProfileSpec.HealthMonitoring = &configv1beta1.HealthMonitoring{
			Enabled: monitoringEnabled,
		}

		return clusterSummary
	}

	// generateGeneratedHealthCheck returns the HealthCheck generated for the ClusterSummary
	generateGeneratedHealthCheck := func(clusterSummary *configv1beta1.ClusterSummary) *libsveltosv1beta1.HealthCheck {
		return &libsveltosv1beta1.HealthCheck{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hm-" + randomString(),
				Labels: map[string]string{
					configv1beta1.HealthMonitoringClusterSummaryUIDLabel: string(clusterSummary.UID),
				},
				Annotations: map[string]string{
					configv1beta1.HealthMonitoringClusterSummaryNamespaceAnnotation: clusterSummary.Namespace,
					configv1beta1.HealthMonitoringClusterSummaryNameAnnotation:      clusterSummary.Name,
				},
			},
		}
	}

	generateHealthCheckReport := func(namespace, clusterName, healthCheckName string,
		clusterType libsveltosv1beta1.ClusterType,
		resourceStatuses ...libsveltosv1beta1.ResourceStatus) *libsveltosv1beta1.HealthCheckReport {

		return &libsveltosv1beta1.HealthCheckReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      randomString(),
				Labels:    libsveltosv1beta1.GetHealthCheckReportLabels(healthCheckName, clusterName, &clusterType),
			},
			Spec: libsveltosv1beta1.HealthCheckReportSpec{
				ClusterNamespace: namespace,
				ClusterName:      clusterName,
				ClusterType:      clusterType,
				HealthCheckName:  healthCheckName,
				ResourceStatuses: resourceStatuses,
			},
		}
	}

	resourceStatus := func(apiVersion, kind, namespace, name string, status libsveltosv1beta1.HealthStatus,
		message string) libsveltosv1beta1.ResourceStatus {

		return libsveltosv1beta1.ResourceStatus{
			ObjectRef:    corev1.ObjectReference{APIVersion: apiVersion, Kind: kind, Namespace: namespace, Name: name},
			HealthStatus: status,
			Message:      message,
		}
	}

	// runUnhealthyResources runs show unhealthy-resources against the given objects and returns what it printed
	runUnhealthyResources := func(filter show.UnhealthyResourcesFilter, objects ...client.Object) string {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayUnhealthyResources(context.TODO(), &filter,
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

	It("show unhealthy-resources lists the unhealthy resources, with the profile owning the HealthCheck", func() {
		clusterSummary := generateHealthClusterSummary(namespace, clusterName, profileName, true)
		healthCheck := generateGeneratedHealthCheck(clusterSummary)
		report := generateHealthCheckReport(namespace, clusterName, healthCheck.Name,
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "frontend", libsveltosv1beta1.HealthStatusDegraded,
				"0/3 replicas available"),
			resourceStatus("v1", "PersistentVolumeClaim", "web", "data", libsveltosv1beta1.HealthStatusProgressing,
				"Pending"),
			resourceStatus("apps/v1", "StatefulSet", "web", "db", libsveltosv1beta1.HealthStatusHealthy, ""))

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{}, clusterSummary, healthCheck, report)

		clusterInfo := namespace + "/" + clusterName
		deploymentLine := findLine(output, clusterInfo, "ClusterProfile/"+profileName, "Deployment.apps", "web",
			"frontend", "Degraded", "0/3 replicas available")
		Expect(deploymentLine).ToNot(BeEmpty(), output)
		// Core group resources are displayed with the kind only
		claimLine := findLine(output, clusterInfo, "ClusterProfile/"+profileName, "PersistentVolumeClaim", "data",
			"Progressing")
		Expect(claimLine).ToNot(BeEmpty(), output)
		Expect(claimLine).ToNot(ContainSubstring("PersistentVolumeClaim."))
		// worst status first
		Expect(strings.Index(output, deploymentLine)).To(BeNumerically("<", strings.Index(output, claimLine)))

		Expect(output).ToNot(ContainSubstring("StatefulSet"))
		Expect(output).ToNot(ContainSubstring("Note:"))
	})

	It("show unhealthy-resources displays the HealthCheck when no profile owns the report", func() {
		healthCheckName := randomString()
		report := generateHealthCheckReport(namespace, clusterName, healthCheckName,
			libsveltosv1beta1.ClusterTypeCapi,
			resourceStatus("apps/v1", "DaemonSet", "kube-system", "agent", libsveltosv1beta1.HealthStatusDegraded, "crash"))

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{}, report)

		Expect(findLine(output, namespace+"/"+clusterName, "HealthCheck/"+healthCheckName, "DaemonSet.apps",
			"agent", "Degraded", "crash")).ToNot(BeEmpty(), output)
		Expect(output).ToNot(ContainSubstring("Note:"))
	})

	It("show unhealthy-resources --profile displays only the resources reported through that profile", func() {
		otherProfileName := randomString()

		clusterSummary := generateHealthClusterSummary(namespace, clusterName, profileName, true)
		healthCheck := generateGeneratedHealthCheck(clusterSummary)
		report := generateHealthCheckReport(namespace, clusterName, healthCheck.Name,
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "mine", libsveltosv1beta1.HealthStatusDegraded, "x"))

		otherSummary := generateHealthClusterSummary(namespace, clusterName, otherProfileName, true)
		otherHealthCheck := generateGeneratedHealthCheck(otherSummary)
		otherReport := generateHealthCheckReport(namespace, clusterName, otherHealthCheck.Name,
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "theirs", libsveltosv1beta1.HealthStatusDegraded, "x"))

		unownedReport := generateHealthCheckReport(namespace, clusterName, randomString(),
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "nobodys", libsveltosv1beta1.HealthStatusDegraded, "x"))

		objects := []client.Object{clusterSummary, healthCheck, report, otherSummary, otherHealthCheck,
			otherReport, unownedReport}

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{Profile: profileName}, objects...)
		Expect(findLine(output, "ClusterProfile/"+profileName, "mine")).ToNot(BeEmpty(), output)
		Expect(output).ToNot(ContainSubstring("theirs"))
		Expect(output).ToNot(ContainSubstring("nobodys"))

		output = runUnhealthyResources(show.UnhealthyResourcesFilter{}, objects...)
		Expect(findLine(output, "mine")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "theirs")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "nobodys")).ToNot(BeEmpty(), output)
	})

	It("show unhealthy-resources filters by cluster namespace, name and type", func() {
		otherNamespace := namePrefix + randomString()
		otherClusterName := randomString()

		report := generateHealthCheckReport(namespace, clusterName, randomString(),
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "first", libsveltosv1beta1.HealthStatusDegraded, "x"))
		sameNamespaceReport := generateHealthCheckReport(namespace, otherClusterName, randomString(),
			libsveltosv1beta1.ClusterTypeCapi,
			resourceStatus("apps/v1", "Deployment", "web", "second", libsveltosv1beta1.HealthStatusDegraded, "x"))
		otherNamespaceReport := generateHealthCheckReport(otherNamespace, clusterName, randomString(),
			libsveltosv1beta1.ClusterTypeSveltos,
			resourceStatus("apps/v1", "Deployment", "web", "third", libsveltosv1beta1.HealthStatusDegraded, "x"))
		objects := []client.Object{report, sameNamespaceReport, otherNamespaceReport}

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{Namespace: namespace}, objects...)
		Expect(output).To(ContainSubstring("first"))
		Expect(output).To(ContainSubstring("second"))
		Expect(output).ToNot(ContainSubstring("third"))

		output = runUnhealthyResources(show.UnhealthyResourcesFilter{Cluster: clusterName}, objects...)
		Expect(output).To(ContainSubstring("first"))
		Expect(output).To(ContainSubstring("third"))
		Expect(output).ToNot(ContainSubstring("second"))

		output = runUnhealthyResources(show.UnhealthyResourcesFilter{ClusterType: "Capi"}, objects...)
		Expect(output).To(ContainSubstring("second"))
		Expect(output).ToNot(ContainSubstring("first"))
		Expect(output).ToNot(ContainSubstring("third"))
	})

	It("show unhealthy-resources explains an empty result: not enabled, no data yet, healthy", func() {
		notEnabledCluster := randomString()
		noDataCluster := randomString()
		healthyCluster := randomString()

		notEnabled := generateHealthClusterSummary(namespace, notEnabledCluster, profileName, false)

		noData := generateHealthClusterSummary(namespace, noDataCluster, profileName, true)
		noDataHealthCheck := generateGeneratedHealthCheck(noData)

		healthy := generateHealthClusterSummary(namespace, healthyCluster, profileName, true)
		healthyHealthCheck := generateGeneratedHealthCheck(healthy)
		healthyReport := generateHealthCheckReport(namespace, healthyCluster, healthyHealthCheck.Name,
			libsveltosv1beta1.ClusterTypeSveltos)

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{}, notEnabled, noData, noDataHealthCheck,
			healthy, healthyHealthCheck, healthyReport)

		Expect(findLine(output, "Note:", namespace+"/"+notEnabledCluster, "not enabled")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "Note:", namespace+"/"+noDataCluster, "no health report")).ToNot(BeEmpty(), output)
		Expect(findLine(output, "Note:", namespace+"/"+healthyCluster, "no unhealthy resources")).ToNot(BeEmpty(), output)
	})

	It("show unhealthy-resources --profile explains the state of that profile", func() {
		noDataProfile := randomString()
		notEnabledProfile := randomString()

		healthy := generateHealthClusterSummary(namespace, clusterName, profileName, true)
		healthyHealthCheck := generateGeneratedHealthCheck(healthy)
		healthyReport := generateHealthCheckReport(namespace, clusterName, healthyHealthCheck.Name,
			libsveltosv1beta1.ClusterTypeSveltos)
		noData := generateHealthClusterSummary(namespace, clusterName, noDataProfile, true)
		noDataHealthCheck := generateGeneratedHealthCheck(noData)
		notEnabled := generateHealthClusterSummary(namespace, clusterName, notEnabledProfile, false)
		objects := []client.Object{healthy, healthyHealthCheck, healthyReport, noData, noDataHealthCheck, notEnabled}

		output := runUnhealthyResources(show.UnhealthyResourcesFilter{Profile: profileName}, objects...)
		Expect(findLine(output, "Note:", "ClusterProfile/"+profileName, "no unhealthy resources")).ToNot(BeEmpty(), output)
		Expect(output).ToNot(ContainSubstring(noDataProfile))
		Expect(output).ToNot(ContainSubstring(notEnabledProfile))

		output = runUnhealthyResources(show.UnhealthyResourcesFilter{Profile: noDataProfile}, objects...)
		Expect(findLine(output, "Note:", "ClusterProfile/"+noDataProfile, "no health report")).ToNot(BeEmpty(), output)

		output = runUnhealthyResources(show.UnhealthyResourcesFilter{Profile: notEnabledProfile}, objects...)
		Expect(findLine(output, "Note:", "ClusterProfile/"+notEnabledProfile, "not enabled")).ToNot(BeEmpty(), output)
	})
})
