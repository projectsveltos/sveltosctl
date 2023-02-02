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
	"path"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Worker", func() {
	BeforeEach(func() {
		snapshotter.InitializeClient(context.TODO(), klogr.New(), nil, 10)
	})

	It("removeFromSlice should remove element from slice", func() {
		tmp := []string{"eng", "sale", "hr"}
		tmp = snapshotter.RemoveFromSlice(tmp, 1)
		Expect(len(tmp)).To(Equal(2))
		Expect(tmp[0]).To(Equal("eng"))
		Expect(tmp[1]).To(Equal("hr"))

		tmp = snapshotter.RemoveFromSlice(tmp, 1)
		Expect(len(tmp)).To(Equal(1))

		tmp = snapshotter.RemoveFromSlice(tmp, 0)
		Expect(len(tmp)).To(Equal(0))
	})

	It("storeResult saves results and removes key from inProgress", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshostName := randomString()
		d.SetInProgress([]string{snapshostName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		snapshotter.StoreResult(d, snapshostName, nil, klogr.New())
		Expect(len(d.GetInProgress())).To(Equal(0))
	})

	It("storeResult saves results and removes key from dirty and adds to jobQueue", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshotName := randomString()
		d.SetInProgress([]string{snapshotName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		d.SetDirty([]string{snapshotName})
		Expect(len(d.GetDirty())).To(Equal(1))

		snapshotter.StoreResult(d, snapshotName, nil, klogr.New())
		Expect(len(d.GetInProgress())).To(Equal(0))
		Expect(len(d.GetDirty())).To(Equal(0))
		Expect(len(d.GetJobQueue())).To(Equal(1))
	})

	It("getRequestStatus returns result when available", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshotName := randomString()

		r := map[string]error{snapshotName: nil}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		resp, err := snapshotter.GetRequestStatus(d, snapshotName)
		Expect(err).To(BeNil())
		Expect(resp).ToNot(BeNil())
		Expect(snapshotter.IsResponseDeployed(resp)).To(BeTrue())
	})

	It("getRequestStatus returns result when available and reports error", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshotName := randomString()

		r := map[string]error{snapshotName: fmt.Errorf("failed to deploy")}
		d.SetResults(r)
		Expect(len(d.GetResults())).To(Equal(1))

		resp, err := snapshotter.GetRequestStatus(d, snapshotName)
		Expect(err).To(BeNil())
		Expect(resp).ToNot(BeNil())
		Expect(snapshotter.IsResponseFailed(resp)).To(BeTrue())
	})

	It("getRequestStatus returns nil response when request is still queued (currently in progress)", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshotName := randomString()

		d.SetInProgress([]string{snapshotName})
		Expect(len(d.GetInProgress())).To(Equal(1))

		resp, err := snapshotter.GetRequestStatus(d, snapshotName)
		Expect(err).To(BeNil())
		Expect(resp).To(BeNil())
	})

	It("getRequestStatus returns nil response when request is still queued (currently queued)", func() {
		d := snapshotter.GetClient()
		defer d.ClearInternalStruct()

		snapshotName := randomString()

		d.SetJobQueue(snapshotName)
		Expect(len(d.GetJobQueue())).To(Equal(1))

		resp, err := snapshotter.GetRequestStatus(d, snapshotName)
		Expect(err).To(BeNil())
		Expect(resp).To(BeNil())
	})

	It("collectSnapshot collects all necessary resources when snapshot runs", func() {
		snapshotInstance := generateSnapshot()
		clusterConfiguration := generateClusterConfiguration()
		clusterProfile := generateClusterProfile()
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: randomString(),
				Name:      randomString(),
			},
		}

		clusterProfile.Spec.PolicyRefs = []libsveltosv1alpha1.PolicyRef{
			{
				Namespace: cm.Namespace,
				Name:      cm.Name,
				Kind:      string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
			},
		}

		Expect(snapshotter.AddTypeInformationToObject(cm)).To(Succeed())
		Expect(snapshotter.AddTypeInformationToObject(clusterProfile)).To(Succeed())
		Expect(snapshotter.AddTypeInformationToObject(clusterConfiguration)).To(Succeed())

		initObjects := []client.Object{clusterConfiguration, clusterProfile, cm, snapshotInstance}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

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
		Expect(c.Update(context.TODO(), snapshotInstance)).To(Succeed())

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		time.Sleep(2 * time.Second)
		err = snapshotter.CollectSnapshot(context.TODO(), c, snapshotInstance.Name, klogr.New())
		Expect(err).To(BeNil())

		baseSnaphostInstanceDir := filepath.Dir(snapshotFolder)
		snapshots, err := os.ReadDir(baseSnaphostInstanceDir)
		Expect(err).To(BeNil())

		// One directory was created when createDirectoryWithClusterConfigurations was called. Second one was created by
		// CollectSnapshot
		Expect(len(snapshots)).To(Equal(2))

		objects := make(map[string]bool)
		objects[objectToString(cm)] = false
		objects[objectToString(clusterProfile)] = false
		objects[objectToString(clusterConfiguration)] = false

		for i := range snapshots {
			recursiveSearchDir(filepath.Join(baseSnaphostInstanceDir, snapshots[i].Name()), objects)
		}

		for o := range objects {
			Expect(objects[o]).To(BeTrue())
		}
	})

	It("dumpClassifiers collects existing classifiers", func() {
		classifier1 := libsveltosv1alpha1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1alpha1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
					{Key: randomString(), Value: randomString()},
				},
			},
		}
		Expect(snapshotter.AddTypeInformationToObject(&classifier1)).To(Succeed())

		classifier2 := libsveltosv1alpha1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1alpha1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1alpha1.ClassifierLabel{
					{Key: randomString(), Value: randomString()},
				},
			},
		}
		Expect(snapshotter.AddTypeInformationToObject(&classifier2)).To(Succeed())

		snapshotInstance := generateSnapshot()

		initObjects := []client.Object{&classifier1, &classifier2, snapshotInstance}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		snapshotFolder := createDirectory(snapshotInstance.Name, snapshotInstance.Spec.Storage)

		Expect(snapshotter.DumpClassifiers(context.TODO(), snapshotFolder, klogr.New())).To(Succeed())

		snapshots, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())

		Expect(len(snapshots)).To(Equal(1))

		objects := make(map[string]bool)
		objects[objectToString(&classifier1)] = false
		objects[objectToString(&classifier2)] = false

		for i := range snapshots {
			recursiveSearchDir(filepath.Join(snapshotFolder, snapshots[i].Name()), objects)
		}

		for o := range objects {
			Expect(objects[o]).To(BeTrue())
		}
	})

	It("dumpRoleRequests collects existing roleRequests and referenced resources", func() {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: randomString(),
				Name:      randomString(),
			},
		}
		Expect(snapshotter.AddTypeInformationToObject(cm)).To(Succeed())

		roleRequest := &libsveltosv1alpha1.RoleRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1alpha1.RoleRequestSpec{
				RoleRefs: []libsveltosv1alpha1.PolicyRef{
					{
						Kind:      string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
						Namespace: cm.GetNamespace(),
						Name:      cm.GetName(),
					},
				},
			},
		}
		Expect(snapshotter.AddTypeInformationToObject(roleRequest)).To(Succeed())

		snapshotInstance := generateSnapshot()

		initObjects := []client.Object{cm, roleRequest, snapshotInstance}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		snapshotFolder := createDirectory(snapshotInstance.Name, snapshotInstance.Spec.Storage)

		Expect(snapshotter.DumpRoleRequests(context.TODO(), snapshotFolder, klogr.New())).To(Succeed())

		snapshots, err := os.ReadDir(snapshotFolder)
		Expect(err).To(BeNil())

		Expect(len(snapshots)).To(Equal(2)) // directory  for RoleRequest plus directory for namespace with ConfigMap

		objects := make(map[string]bool)
		objects[objectToString(cm)] = false
		objects[objectToString(roleRequest)] = false

		for i := range snapshots {
			recursiveSearchDir(filepath.Join(snapshotFolder, snapshots[i].Name()), objects)
		}

		for o := range objects {
			Expect(objects[o]).To(BeTrue())
		}
	})
})

func objectToString(o client.Object) string {
	return fmt.Sprintf("%s:%s/%s", o.GetObjectKind().GroupVersionKind().Kind,
		o.GetNamespace(), o.GetName())
}

func recursiveSearchDir(dir string, objects map[string]bool) {
	files, err := os.ReadDir(dir)
	Expect(err).To(BeNil())

	for i := range files {
		if files[i].IsDir() {
			recursiveSearchDir(path.Join(dir, files[i].Name()), objects)
		} else {
			content, err := os.ReadFile(path.Join(dir, files[i].Name()))
			Expect(err).To(BeNil())
			instance := snapshotter.GetClient()
			u, err := instance.GetUnstructured(content)
			Expect(err).To(BeNil())
			objects[objectToString(u)] = true
		}
	}
}
