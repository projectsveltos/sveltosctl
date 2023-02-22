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
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	timeFormat = "2006-01-02:15:04:05"
)

func TestSnapshot(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshot Suite")
}

func createSnapshotDirectories(snapshotName, snapshotStorage string, numOfDirs int) string {
	snapshotDir, err := os.MkdirTemp("", randomString())
	Expect(err).To(BeNil())
	snapshotDir = filepath.Join(snapshotDir, snapshotStorage)
	Expect(os.Mkdir(snapshotDir, os.ModePerm)).To(Succeed())
	tmpDir := filepath.Join(snapshotDir, "snapshot")
	Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
	tmpDir = filepath.Join(tmpDir, snapshotName)
	Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())

	now := time.Now()
	for i := 0; i < numOfDirs; i++ {
		timeFolder := now.Add(-time.Second * time.Duration(2*i)).Format(timeFormat)
		tmpDir := filepath.Join(snapshotDir, "snapshot", snapshotName, timeFolder)
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
		By(fmt.Sprintf("Created temporary directory %s", tmpDir))
	}

	By(fmt.Sprintf("Snapshot directory: %s", snapshotDir))
	return snapshotDir
}

// createSnapshotDirectoryWithObjects assumes that snapshotStorage/snapshot/snapshotName directory exists.
// Adds a subdirectory <time format> and dumps content of each objects in the format namespace/kind/name.yaml
func createSnapshotDirectoryWithObjects(snapshotName, snapshotStorage string, objects []client.Object) string {
	timeFolder := time.Now().Format(timeFormat)
	snapshotDir := filepath.Join(snapshotStorage, "snapshot", snapshotName, timeFolder)
	Expect(os.Mkdir(snapshotDir, os.ModePerm)).To(Succeed())

	collectorClient := collector.GetClient()

	for i := range objects {
		By(fmt.Sprintf("Dumping object %s %s/%s in folder %s", objects[i].GetObjectKind().GroupVersionKind().Kind,
			objects[i].GetNamespace(), objects[i].GetName(), snapshotDir))
		Expect(collectorClient.DumpObject(objects[i], snapshotDir, klogr.New())).To(Succeed())
	}
	return timeFolder
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}

func addTypeInformationToObject(obj client.Object) error {
	scheme, err := utils.GetScheme()
	if err != nil {
		return err
	}
	// Following are needed by test only
	err = rbacv1.AddToScheme(scheme)
	if err != nil {
		return err
	}

	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}

	for _, gvk := range gvks {
		if gvk.Kind == "" {
			continue
		}
		if gvk.Version == "" || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		break
	}

	return nil
}
