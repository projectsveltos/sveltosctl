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

package snapshotter_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"

	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
)

var (
	clusterConfigurationInstance = `apiVersion: config.projectsveltos.io/v1alpha1
kind: ClusterConfiguration
metadata:
  creationTimestamp: "2022-10-05T22:31:43Z"
  generation: 1
  name: sveltos-management-workload
  namespace: default
  ownerReferences:
  - apiVersion: config.projectsveltos.io/v1alpha1
    kind: ClusterProfile
    name: mgianluc
    uid: 7dbfec81-be91-4e65-a237-f314b72b292b
  resourceVersion: "2250"
  uid: b7606396-1458-4c6e-859d-252af4f25749
status:
  clusterProfileResources:
  - Features:
    - featureID: Resources
    clusterProfileName: mgianluc`
)

var _ = Describe("Client", func() {
	BeforeEach(func() {
		snapshotter.InitializeClient(context.TODO(), klogr.New(), nil, 10)
	})

	It("GetResult returns result when available", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		r := map[string]error{snapshotName: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(snapshotter.Collected))
	})

	It("GetResult returns result when available with error", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		r := map[string]error{snapshotName: fmt.Errorf("failed to deploy")}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName)
		Expect(result.Err).ToNot(BeNil())
		Expect(result.ResultStatus).To(Equal(snapshotter.Failed))
	})

	It("GetResult returns InProgress when request is still queued (currently in progress)", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		d.SetInProgress([]string{snapshotName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(snapshotter.InProgress))
	})

	It("GetResult returns InProgress when request is still queued (currently queued)", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		d.SetJobQueue(snapshotName)
		Expect(len(d.GetJobQueue())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(snapshotter.InProgress))
	})

	It("GetResult returns Unavailable when request is not queued/in progress and result not available", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		result := d.GetResult(context.TODO(), snapshotName)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(snapshotter.Unavailable))
	})

	It("Collect does nothing if already in the dirty set", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		d.SetDirty([]string{snapshotName})
		Expect(len(d.GetDirty())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(0))
	})

	It("Collect adds to inProgress", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		err := d.Collect(context.TODO(), snapshotName)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(1))
	})

	It("Collect if already in progress, does not add to jobQueue", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		d.SetInProgress([]string{snapshotName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(1))
		Expect(len(d.GetJobQueue())).To(Equal(0))
	})

	It("Collect removes existing result", func() {
		snapshotName := randomString()

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		r := map[string]error{snapshotName: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(1))
		Expect(len(d.GetResults())).To(Equal(0))
	})

	It("CleanupEntries removes features from internal data structure but inProgress", func() {
		snapshotName := randomString()
		storageDir, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())

		snapshotDir := filepath.Join(storageDir, snapshotName)

		const permission0755 = 0755
		Expect(os.MkdirAll(snapshotDir, permission0755)).To(Succeed())

		snapshotInstance := &utilsv1alpha1.Snapshot{
			ObjectMeta: metav1.ObjectMeta{
				Name: snapshotName,
			},
			Spec: utilsv1alpha1.SnapshotSpec{
				Storage: storageDir,
			},
		}

		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		r := map[string]error{snapshotName: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		d.SetInProgress([]string{snapshotName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		d.SetDirty([]string{snapshotName})
		Expect(len(d.GetDirty())).To(Equal(1))

		d.SetJobQueue(snapshotName)
		Expect(len(d.GetJobQueue())).To(Equal(1))

		Expect(d.CleanupEntries(snapshotInstance)).To(Succeed())
		Expect(len(d.GetDirty())).To(Equal(0))
		Expect(len(d.GetInProgress())).To(Equal(1))
		Expect(len(d.GetJobQueue())).To(Equal(0))
		Expect(len(d.GetResults())).To(Equal(0))
		_, err = os.Stat(snapshotDir)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("GetUnstructured returns unstructured", func() {
		instance := snapshotter.GetClient()
		u, err := instance.GetUnstructured([]byte(clusterConfigurationInstance))
		Expect(err).To(BeNil())
		Expect(u).ToNot(BeNil())
		Expect(u.GetKind()).To(Equal(configv1alpha1.ClusterConfigurationKind))
	})

	It("getResourcesForKind returns all resources of a given namespaced kind", func() {
		snapshotFolder := createDirectoryWithClusterConfigurations(randomString(), randomString())
		defer os.RemoveAll(snapshotFolder)

		By(fmt.Sprintf("reading content of directory %s", snapshotFolder))
		files, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())
		Expect(len(files)).ToNot(BeZero())

		for i := range files {
			namespaceFolder := filepath.Join(snapshotFolder, files[i].Name())
			By(fmt.Sprintf("finding resources in folder %s", namespaceFolder))
			instance := snapshotter.GetClient()
			list, err := snapshotter.GetResourcesForKind(instance, namespaceFolder, configv1alpha1.ClusterConfigurationKind, klogr.New())
			Expect(err).To(BeNil())
			Expect(list).ToNot(BeNil())
			Expect(len(list)).ToNot(BeZero())
		}
	})

	It("getResourcesForKind returns all resources of a given cluster kind", func() {
		snapshotFolder := createDirectoryWithClusterProfiles(randomString(), randomString())
		defer os.RemoveAll(snapshotFolder)

		By(fmt.Sprintf("reading content of directory %s", snapshotFolder))
		files, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())
		Expect(len(files)).ToNot(BeZero())

		By(fmt.Sprintf("finding resources in folder %s", snapshotFolder))
		instance := snapshotter.GetClient()
		list, err := snapshotter.GetResourcesForKind(instance, snapshotFolder, configv1alpha1.ClusterProfileKind, klogr.New())
		Expect(err).To(BeNil())
		Expect(list).ToNot(BeNil())
		Expect(len(list)).ToNot(BeZero())
	})

	It("GetNamespacedResources returns all resource of a given Kind in a snapshot folder", func() {
		snapshotFolder := createDirectoryWithClusterConfigurations(randomString(), randomString())
		defer os.RemoveAll(snapshotFolder)

		d := snapshotter.GetClient()
		resourceMap, err := d.GetNamespacedResources(snapshotFolder, configv1alpha1.ClusterConfigurationKind,
			klogr.New())
		Expect(err).To(BeNil())
		Expect(resourceMap).ToNot(BeNil())
		Expect(len(resourceMap)).ToNot(BeZero())
		for k := range resourceMap {
			resources := resourceMap[k]
			Expect(len(resources)).ToNot(BeZero())
			for j := range resources {
				u := resources[j]
				Expect(u.GetNamespace()).To(Equal(k))
				Expect(u.GetKind()).To(Equal(configv1alpha1.ClusterConfigurationKind))
			}
		}
	})

	It("ListSnapshots returns all snapshots collected for a given instance", func() {
		snapshotInstance := generateSnapshot()

		By(fmt.Sprintf("snapshot instance %s (storage %s)", snapshotInstance.Name, snapshotInstance.Spec.Storage))

		snapshotFolder1 := createDirectoryWithClusterConfigurations(snapshotInstance.Name, snapshotInstance.Spec.Storage)
		defer os.RemoveAll(snapshotFolder1)

		// createDirectoryWithClusterConfigurations creates a temporary directory and then creates following subdirectories:
		// - snapshotInstance.Spec.Storage
		// - snapshotInstance.Name
		// - timeFolder (directory when time was taken)

		// ListSnapshots expect the directory containing the snapshots for a given Snapshot instance
		// to be: snapshotInstance.Spec.Storage
		// so go up 2 level from snapshotFolder1 (or snapshotFolder2)
		parentUp2 := filepath.Dir(filepath.Dir(snapshotFolder1))
		snapshotInstance.Spec.Storage = parentUp2

		time.Sleep(2 * time.Second) // sleep so timeFolder is different
		timeFolder := time.Now().Format(snapshotter.TimeFormat)
		secondSnapshotFolder := filepath.Join(parentUp2, snapshotInstance.Name, timeFolder)
		cc := generateClusterConfiguration()
		By(fmt.Sprintf("Adding ClusterConfiguration %s/%s to directory %s",
			cc.Namespace, cc.Name, secondSnapshotFolder))
		Expect(snapshotter.AddTypeInformationToObject(cc)).To(Succeed())
		Expect(snapshotter.DumpObject(cc, secondSnapshotFolder, klogr.New())).To(Succeed())

		d := snapshotter.GetClient()
		results, err := d.ListSnapshots(snapshotInstance, klogr.New())
		Expect(err).To(BeNil())
		Expect(len(results)).To(Equal(2))
	})

	It("GetCollectedSnapshotFolder returns the folder for a given Snapshot instance", func() {
		snapshotInstance := generateSnapshot()

		By(fmt.Sprintf("snapshot instance %s (storage %s)", snapshotInstance.Name, snapshotInstance.Spec.Storage))

		snapshotFolder := createDirectoryWithClusterConfigurations(snapshotInstance.Name, snapshotInstance.Spec.Storage)
		defer os.RemoveAll(snapshotFolder)

		// createDirectoryWithClusterConfigurations creates a temporary directory and then creates following subdirectories:
		// - snapshotInstance.Spec.Storage
		// - snapshotInstance.Name
		// - timeFolder (directory when time was taken)

		// ListSnapshots expect the directory containing the snapshots for a given Snapshot instance
		// to be: snapshotInstance.Spec.Storage
		// so go up 2 level from snapshotFolder1 (or snapshotFolder2)
		parentUp2 := filepath.Dir(filepath.Dir(snapshotFolder))
		snapshotInstance.Spec.Storage = parentUp2

		d := snapshotter.GetClient()
		_, err := d.GetCollectedSnapshotFolder(snapshotInstance, klogr.New())
		Expect(err).To(BeNil())
	})
})

func createDirectoryWithClusterConfigurations(snapshotName, snapshotStorage string) string {
	snapshotDir := createDirectory(snapshotName, snapshotStorage)

	By(fmt.Sprintf("Created temporary directory %s", snapshotDir))

	for i := 0; i < 10; i++ {
		cc := generateClusterConfiguration()
		By(fmt.Sprintf("Adding ClusterConfiguration %s/%s to directory %s",
			cc.Namespace, cc.Name, snapshotDir))
		Expect(snapshotter.AddTypeInformationToObject(cc)).To(Succeed())
		Expect(snapshotter.DumpObject(cc, snapshotDir, klogr.New())).To(Succeed())
	}

	return snapshotDir
}

func createDirectoryWithClusterProfiles(snapshotName, snapshotStorage string) string {
	snapshotDir := createDirectory(snapshotName, snapshotStorage)

	By(fmt.Sprintf("Created temporary directory %s", snapshotDir))

	for i := 0; i < 10; i++ {
		cp := generateClusterProfile()
		By(fmt.Sprintf("Adding ClusterProfile %s to directory %s",
			cp.Name, snapshotDir))
		Expect(snapshotter.AddTypeInformationToObject(cp)).To(Succeed())
		Expect(snapshotter.DumpObject(cp, snapshotDir, klogr.New())).To(Succeed())
	}

	return snapshotDir
}

func createDirectory(snapshotName, snapshotStorage string) string {
	snapshotDir, err := os.MkdirTemp("", randomString())
	Expect(err).To(BeNil())
	timeFolder := time.Now().Format(snapshotter.TimeFormat)
	snapshotDir = filepath.Join(snapshotDir, snapshotStorage, snapshotName, timeFolder)
	return snapshotDir
}
