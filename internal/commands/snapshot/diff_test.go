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

package snapshot_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gdexlab/go-render/render"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1alpha1"
	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/commands/snapshot"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Snapshot Diff", func() {
	BeforeEach(func() {
	})

	It("snapshot diff displays all diff between two snapshot collections", func() {
		snapshotInstance := &utilsv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: utilsv1alpha1.SnapshotSpec{
				Storage: randomString(),
			},
		}

		snapshotDir, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())
		snapshotDir = filepath.Join(snapshotDir, snapshotInstance.Spec.Storage)
		Expect(os.Mkdir(snapshotDir, os.ModePerm)).To(Succeed())
		snapshotInstance.Spec.Storage = snapshotDir
		tmpDir := filepath.Join(snapshotDir, "snapshot")
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
		tmpDir = filepath.Join(tmpDir, snapshotInstance.Name)
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())

		clusterConfigurations := generateClusterConfiguration()
		Expect(addTypeInformationToObject(clusterConfigurations)).To(Succeed())

		timeOne := createSnapshotDirectoryWithObjects(snapshotInstance.Name, snapshotInstance.Spec.Storage,
			[]client.Object{clusterConfigurations})

		time.Sleep(2 * time.Second) // wait so to simulate a snapshot at a different time

		clusterConfigurations.Status.ClusterProfileResources =
			append(clusterConfigurations.Status.ClusterProfileResources, *generateClusterProfileResource())
		timeTwo := createSnapshotDirectoryWithObjects(snapshotInstance.Name, snapshotInstance.Spec.Storage,
			[]client.Object{clusterConfigurations})

		initObjects := []client.Object{snapshotInstance}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		collector.InitializeClient(context.TODO(), klogr.New(), c, 10)

		err = snapshot.ListSnapshotDiffs(context.TODO(), snapshotInstance.Name, timeOne, timeTwo, "", "", false, klogr.New())
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// Example of expected result
			+-----------------------+-----------------------+------------+------------+--------+---------+
			|        CLUSTER        |     RESOURCE TYPE     | NAMESPACE  |    NAME    | ACTION | MESSAGE |
			+-----------------------+-----------------------+------------+------------+--------+---------+
			| 7hg5dkvkm8/wbvgz9b3zw | helm release          | eerbkvgnlx | aac4u5x73i | added  |         |
			| 7hg5dkvkm8/wbvgz9b3zw | helm release          | av2tmp8naj | invzb9koy7 | added  |         |
			| 7hg5dkvkm8/wbvgz9b3zw | g6fc23g1r5/xlca7ykzjr | r8m35supij | 17ti1oh75g | added  |         |
			| 7hg5dkvkm8/wbvgz9b3zw | vr6pxt54vd/tdzmgnqy4j | nne00buls6 | j1h5sgqdht | added  |         |
			+-----------------------+-----------------------+------------+------------+--------+---------+
		*/

		clusterInfo := fmt.Sprintf("%s/%s", clusterConfigurations.Namespace, clusterConfigurations.Name)
		helmReleaseAdded := 0
		resourceAdded := 0
		lines := strings.Split(buf.String(), "\n")
		for i := range lines {
			if strings.Contains(lines[i], clusterInfo) &&
				strings.Contains(lines[i], "helm release") &&
				strings.Contains(lines[i], "added") {
				helmReleaseAdded++
			} else if strings.Contains(lines[i], clusterInfo) &&
				strings.Contains(lines[i], "added") {
				resourceAdded++
			}
		}

		Expect(resourceAdded).To(Equal(2))
		Expect(helmReleaseAdded).To(Equal(2))

		os.Stdout = old
	})

	It("listdiff displays all diff between Classifiers/RoleRequests", func() {
		snapshotInstance := &utilsv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: utilsv1alpha1.SnapshotSpec{
				Storage: randomString(),
			},
		}

		snapshotDir, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())
		snapshotDir = filepath.Join(snapshotDir, snapshotInstance.Spec.Storage)
		Expect(os.Mkdir(snapshotDir, os.ModePerm)).To(Succeed())
		snapshotInstance.Spec.Storage = snapshotDir
		tmpDir := filepath.Join(snapshotDir, "snapshot")
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
		tmpDir = filepath.Join(tmpDir, snapshotInstance.Name)
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())

		classifierName := randomString()
		classifier := &libsveltosv1alpha1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: classifierName,
			},
			Spec: libsveltosv1alpha1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
					{Key: randomString(), Value: randomString()},
				},
			},
		}
		Expect(addTypeInformationToObject(classifier)).To(Succeed())

		roleRequest := &libsveltosv1alpha1.RoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: classifierName,
			},
			Spec: libsveltosv1alpha1.RoleRequestSpec{
				ServiceAccountNamespace: randomString(),
				ServiceAccountName:      randomString(),
				ClusterSelector:         libsveltosv1alpha1.Selector("zone:west"),
			},
		}
		Expect(addTypeInformationToObject(roleRequest)).To(Succeed())

		classifierDeleted := &libsveltosv1alpha1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1alpha1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
					{Key: randomString(), Value: randomString()},
				},
			},
		}
		Expect(addTypeInformationToObject(classifierDeleted)).To(Succeed())

		timeOne := createSnapshotDirectoryWithObjects(snapshotInstance.Name, snapshotInstance.Spec.Storage,
			[]client.Object{classifier, classifierDeleted, roleRequest})

		time.Sleep(2 * time.Second) // wait so to simulate a snapshot at a different time

		classifier.Spec.ClassifierLabels =
			append(classifier.Spec.ClassifierLabels, libsveltosv1alpha1.ClassifierLabel{Key: randomString(), Value: randomString()})
		classifierAdded := &libsveltosv1alpha1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1alpha1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
					{Key: randomString(), Value: randomString()},
				},
			},
		}
		Expect(addTypeInformationToObject(classifierAdded)).To(Succeed())

		timeTwo := createSnapshotDirectoryWithObjects(snapshotInstance.Name, snapshotInstance.Spec.Storage,
			[]client.Object{classifier, classifierAdded})

		initObjects := []client.Object{snapshotInstance}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		collector.InitializeClient(context.TODO(), klogr.New(), c, 10)

		snapshotClient := collector.GetClient()
		artifactFolder, err := snapshotClient.GetFolder(snapshotInstance.Spec.Storage, snapshotInstance.Name, collector.Snapshot, klogr.New())
		Expect(err).To(BeNil())
		fromFolder := filepath.Join(*artifactFolder, timeOne)
		toFolder := filepath.Join(*artifactFolder, timeTwo)

		err = snapshot.ListDiff(fromFolder, toFolder, libsveltosv1alpha1.ClassifierKind, false, klogr.New())
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
				  // Example of expected result
				  +------------+----------+
			      | CLASSIFIER |  ACTION  |
			      +------------+----------+
			      | 06fke9vezz | modified |
			      | i2ngq0c4wk | added    |
			      | srtettp0qu | removed  |
			      +------------+----------+
		*/

		modified, added, deleted := false, false, false

		lines := strings.Split(buf.String(), "\n")
		for i := range lines {
			if strings.Contains(lines[i], classifierName) &&
				strings.Contains(lines[i], "modified") {
				modified = true
			} else if strings.Contains(lines[i], classifierDeleted.Name) &&
				strings.Contains(lines[i], "removed") {
				deleted = true
			} else if strings.Contains(lines[i], classifierAdded.Name) &&
				strings.Contains(lines[i], "added") {
				added = true
			}
		}

		Expect(modified).To(BeTrue())
		Expect(added).To(BeTrue())
		Expect(deleted).To(BeTrue())
		os.Stdout = old

		r, w, _ = os.Pipe()
		os.Stdout = w

		err = snapshot.ListDiff(fromFolder, toFolder, libsveltosv1alpha1.RoleRequestKind, false, klogr.New())
		Expect(err).To(BeNil())

		w.Close()
		var buf1 bytes.Buffer
		_, err = io.Copy(&buf1, r)
		Expect(err).To(BeNil())

		/*
					 	// Example of expected result
						+-------------+---------+
			      		| ROLEREQUEST | ACTION  |
			      		+-------------+---------+
			      		| qr7okih6s9  | removed |
			      		+-------------+---------+
		*/

		deleted = false

		lines = strings.Split(buf1.String(), "\n")
		for i := range lines {
			if strings.Contains(lines[i], roleRequest.Name) &&
				strings.Contains(lines[i], "removed") {
				deleted = true
			}
		}

		Expect(deleted).To(BeTrue())

		os.Stdout = old
	})

	It("listFeaturesDiffInCluster list differences in collected ClusterConfigurations", func() {
		newClusterConfiguration := generateClusterConfiguration()
		oldClusterConfiguration := &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newClusterConfiguration.Name,
				Namespace: newClusterConfiguration.Namespace,
			},
		}
		table := tablewriter.NewWriter(os.Stdout)
		Expect(snapshot.ListDiffInClusterConfigurations("", "",
			[]*configv1alpha1.ClusterConfiguration{oldClusterConfiguration},
			[]*configv1alpha1.ClusterConfiguration{newClusterConfiguration},
			false, table, klogr.New())).To(Succeed())

		result := fmt.Sprintf("table: %v\n", table)
		for i := range newClusterConfiguration.Status.ClusterProfileResources {
			verifyChartAndResources(result, newClusterConfiguration.Status.ClusterProfileResources[i])
		}
	})

	It("listClusterConfigurationDiff list differences between two ClusterConfigurations", func() {
		newClusterConfiguration := generateClusterConfiguration()
		newClusterConfiguration.Status.ClusterProfileResources =
			[]configv1alpha1.ClusterProfileResource{
				*generateClusterProfileResource(),
				*generateClusterProfileResource(),
			}

		oldClusterConfiguration := &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      newClusterConfiguration.Name,
				Namespace: newClusterConfiguration.Namespace,
			},
		}
		table := tablewriter.NewWriter(os.Stdout)
		Expect(snapshot.ListClusterConfigurationDiff("", "", oldClusterConfiguration,
			newClusterConfiguration, false, table, klogr.New())).To(Succeed())

		result := fmt.Sprintf("table: %v\n", table)
		for i := range newClusterConfiguration.Status.ClusterProfileResources {
			verifyChartAndResources(result, newClusterConfiguration.Status.ClusterProfileResources[i])
		}
	})

	It("chartDifference lists added and deleted charts between two slices of charts", func() {
		oldCharts := []configv1alpha1.Chart{*generateChart()}
		newCharts := []configv1alpha1.Chart{*generateChart()}

		added, modified, deleted, message := snapshot.ChartDifference(oldCharts, newCharts)
		Expect(len(added)).To(Equal(1))
		Expect(len(modified)).To(Equal(0))
		Expect(len(deleted)).To(Equal(1))
		Expect(len(message)).To(Equal(0))
	})

	It("chartDifference lists modifed charts between two slices of charts", func() {
		chart := generateChart()
		oldCharts := []configv1alpha1.Chart{*chart}
		newChart := *chart
		newChart.ChartVersion = randomString()
		newCharts := []configv1alpha1.Chart{newChart}

		added, modified, deleted, message := snapshot.ChartDifference(oldCharts, newCharts)
		Expect(len(added)).To(Equal(0))
		Expect(len(modified)).To(Equal(1))
		Expect(len(deleted)).To(Equal(0))
		Expect(len(message)).To(Equal(1))
		Expect(message[newChart]).To(
			Equal(fmt.Sprintf("To version: %s From version %s",
				newChart.ChartVersion, chart.ChartVersion)))
	})

	It("resourceDifference lists added and delete resources between two slices of resources", func() {
		oldResources := []configv1alpha1.Resource{*generateResource()}
		newResources := []configv1alpha1.Resource{*generateResource(), *generateResource()}

		added, modified, deleted, err :=
			snapshot.ResourceDifference("", "", oldResources, newResources, false, klogr.New())
		Expect(err).To(BeNil())
		Expect(len(added)).To(Equal(2))
		Expect(len(modified)).To(Equal(0))
		Expect(len(deleted)).To(Equal(1))
	})

	It("resourceDifference lists modified resources between two slices of resources", func() {
		name := randomString()
		namespace := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		collector.InitializeClient(context.TODO(), klogr.New(), c, 10)

		collectorClient := collector.GetClient()

		oldFolder, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())
		clusterRole := getClusterRole()
		oldConfigMap := createConfigMapWithPolicy(namespace, name, render.AsCode(clusterRole))
		Expect(collectorClient.DumpObject(oldConfigMap, oldFolder, klogr.New())).To(Succeed())

		newFolder, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())
		clusterRole.Rules = []rbacv1.PolicyRule{
			{Verbs: []string{"create", "get"}, APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificaterequests"}},
		}
		newConfigMap := createConfigMapWithPolicy(namespace, name, render.AsCode(clusterRole))
		Expect(collectorClient.DumpObject(newConfigMap, newFolder, klogr.New())).To(Succeed())

		oldResource := &configv1alpha1.Resource{
			Kind:            "ClusterRole",
			Name:            clusterRole.GetName(),
			LastAppliedTime: &metav1.Time{Time: time.Now()},
			Owner: corev1.ObjectReference{
				Kind:      string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
				Name:      oldConfigMap.Name,
				Namespace: oldConfigMap.Namespace,
			},
		}

		newResource := *oldResource
		newResource.LastAppliedTime = &metav1.Time{Time: time.Now().Add(time.Second * time.Duration(2))}

		added, modified, deleted, err :=
			snapshot.ResourceDifference(oldFolder, newFolder, []configv1alpha1.Resource{*oldResource},
				[]configv1alpha1.Resource{newResource}, false, klogr.New())
		Expect(err).To(BeNil())
		Expect(len(added)).To(Equal(0))
		Expect(len(deleted)).To(Equal(0))
		Expect(len(modified)).To(Equal(1))
	})

	It("addChartEntry adds chart entries to table", func() {
		clusterConfiguration := &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}

		charts := make([]*configv1alpha1.Chart, 0)
		charts = append(charts, generateChart())
		charts = append(charts, generateChart())

		table := tablewriter.NewWriter(os.Stdout)
		snapshot.AddChartEntry(nil, clusterConfiguration, charts, "added", nil, table)

		Expect(table.NumLines()).To(Equal(2))
		result := fmt.Sprintf("table: %v\n", table)
		Expect(result).To(ContainSubstring(charts[0].Namespace))
		Expect(result).To(ContainSubstring(charts[0].ReleaseName))
		Expect(result).To(ContainSubstring(charts[1].Namespace))
		Expect(result).To(ContainSubstring(charts[1].ReleaseName))
	})

	It("addResourceEntry adds resource entries to table", func() {
		clusterConfiguration := &configv1alpha1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}

		resources := make([]*configv1alpha1.Resource, 0)
		resources = append(resources, generateResource())
		resources = append(resources, generateResource())

		table := tablewriter.NewWriter(os.Stdout)
		snapshot.AddResourceEntry(nil, clusterConfiguration, resources, "added", "", table)

		Expect(table.NumLines()).To(Equal(2))
		result := fmt.Sprintf("table: %v\n", table)
		Expect(result).To(ContainSubstring(resources[0].Namespace))
		Expect(result).To(ContainSubstring(resources[0].Name))
		Expect(result).To(ContainSubstring(resources[1].Namespace))
		Expect(result).To(ContainSubstring(resources[1].Name))
	})

	It("appendChartsAndResources copies charts and features from CLusterProfileFeature to respective slices", func() {
		cpr := generateClusterProfileResource()
		Expect(cpr.Features).ToNot(BeNil())

		chartNumber := 0
		resourceNumber := 0
		for i := range cpr.Features {
			if cpr.Features[i].Charts != nil {
				chartNumber += len(cpr.Features[i].Charts)
			}
			if cpr.Features[i].Resources != nil {
				resourceNumber += len(cpr.Features[i].Resources)
			}
		}

		Expect(chartNumber).ToNot(BeZero())
		Expect(resourceNumber).ToNot(BeZero())

		charts := make([]configv1alpha1.Chart, 0)
		resources := make([]configv1alpha1.Resource, 0)
		charts, resources = snapshot.AppendChartsAndResources(cpr, charts, resources)

		Expect(len(charts)).To(Equal(chartNumber))
		Expect(len(resources)).To(Equal(resourceNumber))
	})

	It("getResourceFromResourceOwner returns the resource contained in the Owner", func() {
		folder, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())
		By(fmt.Sprintf("using folder %s", folder))
		clusterRole := getClusterRole()

		Expect(addTypeInformationToObject(clusterRole)).To(Succeed())

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		collector.InitializeClient(context.TODO(), klogr.New(), c, 10)
		collectorClient := collector.GetClient()

		name := randomString()
		namespace := randomString()
		configMap := createConfigMapWithPolicy(namespace, name, render.AsCode(clusterRole))
		Expect(collectorClient.DumpObject(configMap, folder, klogr.New())).To(Succeed())
		By(fmt.Sprintf("Dumped ConfigMap %s/%s", configMap.Namespace, configMap.Name))

		clusterRoleGroup := "rbac.authorization.k8s.io/v1"
		clusterRoleKind := "ClusterRole"
		resource := &configv1alpha1.Resource{
			Name:  clusterRole.Name,
			Group: clusterRoleGroup,
			Kind:  clusterRoleKind,
			Owner: corev1.ObjectReference{
				Kind:      string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
				Name:      configMap.Name,
				Namespace: configMap.Namespace,
			},
		}

		policy, err := snapshot.GetResourceFromResourceOwner(folder, resource)
		Expect(err).To(BeNil())
		Expect(policy).To(ContainSubstring(clusterRole.Name))
		Expect(policy).To(ContainSubstring(clusterRoleGroup))
		Expect(policy).To(ContainSubstring(clusterRoleKind))
	})
})

func generateChart() *configv1alpha1.Chart {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Chart{
		RepoURL:         randomString(),
		ReleaseName:     randomString(),
		Namespace:       randomString(),
		ChartVersion:    randomString(),
		LastAppliedTime: &t,
	}
}

func generateResource() *configv1alpha1.Resource {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Resource{
		Name:            randomString(),
		Namespace:       randomString(),
		Group:           randomString(),
		Kind:            randomString(),
		LastAppliedTime: &t,
		Owner: corev1.ObjectReference{
			Kind:      "ConfigMap",
			Namespace: randomString(),
			Name:      randomString(),
		},
	}
}

func generateClusterConfiguration() *configv1alpha1.ClusterConfiguration {
	return &configv1alpha1.ClusterConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      randomString(),
			Namespace: randomString(),
		},
		Status: configv1alpha1.ClusterConfigurationStatus{
			ClusterProfileResources: []configv1alpha1.ClusterProfileResource{
				*generateClusterProfileResource(),
			},
		},
	}
}

func generateClusterProfileResource() *configv1alpha1.ClusterProfileResource {
	return &configv1alpha1.ClusterProfileResource{
		ClusterProfileName: randomString(),
		Features: []configv1alpha1.Feature{
			{
				FeatureID: configv1alpha1.FeatureHelm,
				Charts: []configv1alpha1.Chart{
					*generateChart(), *generateChart(),
				},
			},
			{
				FeatureID: configv1alpha1.FeatureResources,
				Resources: []configv1alpha1.Resource{
					*generateResource(), *generateResource(),
				},
			},
		},
	}
}

func verifyChartAndResources(result string, cpr configv1alpha1.ClusterProfileResource) {
	for i := range cpr.Features {
		if cpr.Features[i].Charts != nil {
			verifyCharts(result, cpr.Features[i].Charts)
		}
		if cpr.Features[i].Resources != nil {
			verifyResources(result, cpr.Features[i].Resources)
		}
	}
}

func verifyCharts(result string, charts []configv1alpha1.Chart) {
	for i := range charts {
		verifyChart(result, &charts[i])
	}
}

func verifyChart(result string, chart *configv1alpha1.Chart) {
	found := false
	lines := strings.Split(result, "\n")
	for i := range lines {
		if strings.Contains(lines[i], chart.Namespace) &&
			strings.Contains(lines[i], chart.ReleaseName) {

			found = true
			break
		}
	}

	Expect(found).To(BeTrue())
}

func verifyResources(result string, resources []configv1alpha1.Resource) {
	for i := range resources {
		verifyResource(result, &resources[i])
	}
}

func verifyResource(result string, resources *configv1alpha1.Resource) {
	found := false
	lines := strings.Split(result, "\n")
	for i := range lines {
		if strings.Contains(lines[i], resources.Namespace) &&
			strings.Contains(lines[i], resources.Name) {

			found = true
			break
		}
	}

	Expect(found).To(BeTrue())
}

// createConfigMapWithPolicy creates a configMap with Data policies
func createConfigMapWithPolicy(namespace, configMapName string, policyStrs ...string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      configMapName,
		},
		Data: map[string]string{},
	}
	for i := range policyStrs {
		key := fmt.Sprintf("policy%d.yaml", i)
		if utf8.Valid([]byte(policyStrs[i])) {
			cm.Data[key] = policyStrs[i]
		} else {
			cm.BinaryData[key] = []byte(policyStrs[i])
		}
	}

	Expect(addTypeInformationToObject(cm)).To(Succeed())

	return cm
}

func getClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomString(),
		},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"create", "get"}, APIGroups: []string{"cert-manager.io"}, Resources: []string{"certificaterequests"}},
			{Verbs: []string{"create", "delete"}, APIGroups: []string{""}, Resources: []string{"namespaces", "deployments"}},
		},
	}
}
