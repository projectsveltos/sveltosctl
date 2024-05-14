/*
Copyright 2022-23. projectsveltos.io. All rights reserved.

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

package collector_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2/textlogger"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
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
		collector.InitializeClient(context.TODO(),
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))), nil, 10)
	})

	It("GetResult returns result when available", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		r := map[string]error{collector.GetKey(snapshotName, collector.Snapshot): nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName, collector.Snapshot)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.Collected))
	})

	It("GetResult returns result when available with error", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(snapshotName, collector.Snapshot)
		r := map[string]error{key: fmt.Errorf("failed to deploy")}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName, collector.Snapshot)
		Expect(result.Err).ToNot(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.Failed))
	})

	It("GetResult returns InProgress when request is still queued (currently in progress)", func() {
		techsupportName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(techsupportName, collector.Techsupport)
		d.SetInProgress([]string{key})
		Expect(len(d.GetInProgress())).To(Equal(1))

		result := d.GetResult(context.TODO(), techsupportName, collector.Techsupport)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.InProgress))
	})

	It("GetResult returns InProgress when request is still queued (currently queued)", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		d.SetJobQueue(snapshotName, collector.Snapshot)
		Expect(len(d.GetJobQueue())).To(Equal(1))

		result := d.GetResult(context.TODO(), snapshotName, collector.Snapshot)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.InProgress))
	})

	It("GetResult returns Unavailable when request is not queued/in progress and result not available", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		result := d.GetResult(context.TODO(), snapshotName, collector.Snapshot)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.Unavailable))

		techsupportName := randomString()
		result = d.GetResult(context.TODO(), techsupportName, collector.Techsupport)
		Expect(result.Err).To(BeNil())
		Expect(result.ResultStatus).To(Equal(collector.Unavailable))
	})

	It("Collect does nothing if already in the dirty set", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(snapshotName, collector.Snapshot)
		d.SetDirty([]string{key})
		Expect(len(d.GetDirty())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName, collector.Snapshot, nil)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(0))
	})

	It("Collect adds to inProgress", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		err := d.Collect(context.TODO(), snapshotName, collector.Snapshot, nil)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(1))
	})

	It("Collect if already in progress, does not add to jobQueue", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(snapshotName, collector.Snapshot)
		d.SetInProgress([]string{key})
		Expect(len(d.GetInProgress())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName, collector.Snapshot, nil)
		Expect(err).To(BeNil())
		Expect(len(d.GetDirty())).To(Equal(1))
		Expect(len(d.GetInProgress())).To(Equal(1))
		Expect(len(d.GetJobQueue())).To(Equal(0))
	})

	It("Collect removes existing result", func() {
		snapshotName := randomString()

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(snapshotName, collector.Snapshot)
		r := map[string]error{key: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		err := d.Collect(context.TODO(), snapshotName, collector.Snapshot, nil)
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

		snapshotDir := filepath.Join(storageDir, "snapshot", snapshotName)

		const permission0755 = 0755
		Expect(os.MkdirAll(snapshotDir, permission0755)).To(Succeed())

		d := collector.GetClient()
		defer d.ClearInternalStruct()

		key := collector.GetKey(snapshotName, collector.Snapshot)
		r := map[string]error{key: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		d.SetInProgress([]string{key})
		Expect(len(d.GetInProgress())).To(Equal(1))

		d.SetDirty([]string{key})
		Expect(len(d.GetDirty())).To(Equal(1))

		d.SetJobQueue(snapshotName, collector.Snapshot)
		Expect(len(d.GetJobQueue())).To(Equal(1))

		Expect(d.CleanupEntries(storageDir, snapshotName, collector.Snapshot)).To(Succeed())
		Expect(len(d.GetDirty())).To(Equal(0))
		Expect(len(d.GetInProgress())).To(Equal(1))
		Expect(len(d.GetJobQueue())).To(Equal(0))
		Expect(len(d.GetResults())).To(Equal(0))
		_, err = os.Stat(snapshotDir)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("GetUnstructured returns unstructured", func() {
		instance := collector.GetClient()
		u, err := instance.GetUnstructured([]byte(clusterConfigurationInstance))
		Expect(err).To(BeNil())
		Expect(u).ToNot(BeNil())
		Expect(u.GetKind()).To(Equal(configv1alpha1.ClusterConfigurationKind))
	})

	It("getResourcesForKind returns all resources of a given namespaced kind", func() {
		snapshotFolder := createDirectoryWithClusterConfigurations(randomString(), randomString(), collector.Techsupport)
		defer os.RemoveAll(snapshotFolder)

		By(fmt.Sprintf("reading content of directory %s", snapshotFolder))
		files, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())
		Expect(len(files)).ToNot(BeZero())

		for i := range files {
			namespaceFolder := filepath.Join(snapshotFolder, files[i].Name())
			By(fmt.Sprintf("finding resources in folder %s", namespaceFolder))
			instance := collector.GetClient()
			list, err := collector.GetResourcesForKind(instance, namespaceFolder, configv1alpha1.ClusterConfigurationKind,
				textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
			Expect(err).To(BeNil())
			Expect(list).ToNot(BeNil())
			Expect(len(list)).ToNot(BeZero())
		}
	})

	It("getResourcesForKind returns all resources of a given cluster kind", func() {
		snapshotFolder := createDirectoryWithClusterProfiles(randomString(), randomString(), collector.Snapshot)
		defer os.RemoveAll(snapshotFolder)

		By(fmt.Sprintf("reading content of directory %s", snapshotFolder))
		files, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())
		Expect(len(files)).ToNot(BeZero())

		By(fmt.Sprintf("finding resources in folder %s", snapshotFolder))
		instance := collector.GetClient()
		list, err := collector.GetResourcesForKind(instance, snapshotFolder, configv1alpha1.ClusterProfileKind,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(list).ToNot(BeNil())
		Expect(len(list)).ToNot(BeZero())
	})

	It("GetNamespacedResources returns all resource of a given Kind in a snapshot folder", func() {
		snapshotFolder := createDirectoryWithClusterConfigurations(randomString(), randomString(), collector.Snapshot)
		defer os.RemoveAll(snapshotFolder)

		d := collector.GetClient()
		resourceMap, err := d.GetNamespacedResources(snapshotFolder, configv1alpha1.ClusterConfigurationKind,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
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

	It("ListCollections returns all collections for a given instance", func() {
		snapshotName := randomString()
		snapshotStorage := randomString()
		By(fmt.Sprintf("snapshot instance %s (storage %s)", snapshotName, snapshotStorage))

		snapshotFolder := createDirectoryWithClusterConfigurations(snapshotStorage, snapshotName, collector.Snapshot)
		defer os.RemoveAll(snapshotFolder)

		// createDirectoryWithClusterConfigurations creates a temporary directory and then creates following subdirectories:
		// - storage/snapshot/
		// - snapshotName
		// - timeFolder (directory when time was taken)

		// ListCollections expect the directory containing the snapshots for a given Snapshot instance
		// to be: storage/snapshot/snapshotName
		// so go up 1 level from snapshotFolder
		parentUp := filepath.Dir(snapshotFolder)

		d := collector.GetClient()

		time.Sleep(2 * time.Second) // sleep so timeFolder is different
		timeFolder := time.Now().Format(collector.TimeFormat)
		secondSnapshotFolder := filepath.Join(parentUp, timeFolder)
		cc := generateClusterConfiguration()
		By(fmt.Sprintf("Adding ClusterConfiguration %s/%s to directory %s",
			cc.Namespace, cc.Name, secondSnapshotFolder))
		Expect(collector.AddTypeInformationToObject(cc)).To(Succeed())
		Expect(d.DumpObject(cc, secondSnapshotFolder,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		// Set storage properly for ListCollection
		storage := filepath.Dir(filepath.Dir(filepath.Dir(snapshotFolder)))
		results, err := d.ListCollections(storage, snapshotName, collector.Snapshot,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(results)).To(Equal(2))
	})

	It("GetFolder returns the folder for a given requestor instance", func() {
		snapshotName := randomString()
		storage := randomString()
		By(fmt.Sprintf("snapshot instance %s (storage %s)", snapshotName, storage))

		snapshotFolder := createDirectoryWithClusterConfigurations(storage, snapshotName, collector.Snapshot)
		defer os.RemoveAll(snapshotFolder)

		// createDirectoryWithClusterConfigurations creates a temporary directory and then creates following subdirectories:
		// - storage/snapshot/
		// - snapshotName
		// - timeFolder (directory when time was taken)

		// ListSnapshots expect the directory containing the snapshots for a given Snapshot instance
		// to be: storage/snapshot/snapshotName
		// so go up 3 level from snapshotFolder
		parentUp3 := filepath.Dir(filepath.Dir(filepath.Dir(snapshotFolder)))
		storage = parentUp3

		d := collector.GetClient()
		_, err := d.GetFolder(storage, snapshotName, collector.Snapshot,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
	})
})

func createDirectoryWithClusterConfigurations(storage, requestorName string, collectionType collector.CollectionType) string {
	snapshotDir := createDirectory(storage, requestorName, collectionType)

	By(fmt.Sprintf("Created temporary directory %s", snapshotDir))
	d := collector.GetClient()

	for i := 0; i < 10; i++ {
		cc := generateClusterConfiguration()
		By(fmt.Sprintf("Adding ClusterConfiguration %s/%s to directory %s",
			cc.Namespace, cc.Name, snapshotDir))
		Expect(collector.AddTypeInformationToObject(cc)).To(Succeed())
		Expect(d.DumpObject(cc, snapshotDir,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	}

	return snapshotDir
}

func createDirectoryWithClusterProfiles(storage, requestorName string, collectionType collector.CollectionType) string {
	snapshotDir := createDirectory(storage, requestorName, collectionType)

	By(fmt.Sprintf("Created temporary directory %s", snapshotDir))
	d := collector.GetClient()
	for i := 0; i < 10; i++ {
		cp := generateClusterProfile()
		By(fmt.Sprintf("Adding ClusterProfile %s to directory %s",
			cp.Name, snapshotDir))
		Expect(collector.AddTypeInformationToObject(cp)).To(Succeed())
		Expect(d.DumpObject(cp, snapshotDir,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	}

	return snapshotDir
}

func createDirectory(storage, requestorName string, collectionType collector.CollectionType) string {
	dir, err := os.MkdirTemp("", randomString())
	Expect(err).To(BeNil())
	timeFolder := time.Now().Format(collector.TimeFormat)

	if collectionType == collector.Snapshot {
		return filepath.Join(dir, storage, "snapshot", requestorName, timeFolder)
	}
	return filepath.Join(dir, storage, "techsupport", requestorName, timeFolder)
}
