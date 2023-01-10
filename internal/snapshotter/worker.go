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

package snapshotter

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// A "request" represents the need to take a snapshot.
//
// When a request arrives, the flow is following:
// - when a request to take a snapshot arrives, it is first added to the dirty
// set or dropped if it already present in the dirty set;
// - pushed to the jobQueue only if it is not presented in inProgress (we don't want
// to take two snapshots in parallel)
//
// When a worker is ready to serve a request, it gets the request from the
// front of the jobQueue.
// The request is also added to the inProgress set and removed from the dirty set.
//
// If a request, currently in the inProgress arrives again, such request is only added
// to the dirty set, not to the queue. This guarantees that a request to take a snapshot
// is never process more than once in parallel.
//
// When worker is done, the request is removed from the inProgress set.
// If the same request is also present in the dirty set, it is added back to the back of the jobQueue.

type requestParams struct {
	key string
}

type responseParams struct {
	requestParams
	err error
}

var (
	controlClusterClient client.Client
)

const (
	permission0600 = 0600
	permission0644 = 0644
	permission0755 = 0755

	timeFormat = "2006-01-02:15:04:05"
)

// startWorkloadWorkers initializes all internal structures and starts
// pool of workers
// - numWorker is number of requested workers
// - c is the kubernetes client to access control cluster
func (d *deployer) startWorkloadWorkers(ctx context.Context, numOfWorker int, logger logr.Logger) {
	d.mu = &sync.Mutex{}
	d.dirty = make([]string, 0)
	d.inProgress = make([]string, 0)
	d.jobQueue = make([]requestParams, 0)
	d.results = make(map[string]error)
	controlClusterClient = d.Client

	for i := 0; i < numOfWorker; i++ {
		go processRequests(ctx, d, i, logger.WithValues("worker", fmt.Sprintf("%d", i)))
	}
}

func processRequests(ctx context.Context, d *deployer, i int, logger logr.Logger) {
	id := i
	var params *requestParams

	logger.V(logs.LogDebug).Info(fmt.Sprintf("started worker %d", id))

	for {
		if params != nil {
			l := logger.WithValues("key", params.key)
			// Get error only from getIsCleanupFromKey as same key is always used
			l.Info(fmt.Sprintf("worker: %d processing request for snapshot %s", id, params.key))
			err := collectSnapshot(ctx, controlClusterClient, params.key, l)
			storeResult(d, params.key, err, l)
		}
		params = nil
		select {
		case <-time.After(1 * time.Second):
			d.mu.Lock()
			if len(d.jobQueue) > 0 {
				// take a request from queue and remove it from queue
				params = &requestParams{key: d.jobQueue[0].key}
				d.jobQueue = d.jobQueue[1:]
				l := logger.WithValues("key", params.key)
				l.V(logs.LogDebug).Info("take from jobQueue")
				// Add to inProgress
				l.V(logs.LogDebug).Info("add to inProgress")
				d.inProgress = append(d.inProgress, params.key)
				// If present remove from dirty
				for i := range d.dirty {
					if d.dirty[i] == params.key {
						l.V(logs.LogDebug).Info("remove from dirty")
						d.dirty = removeFromSlice(d.dirty, i)
						break
					}
				}
			}
			d.mu.Unlock()
		case <-ctx.Done():
			logger.V(logs.LogDebug).Info("context canceled")
			return
		}
	}
}

// doneProcessing does following:
// - set results for further in time lookup
// - remove key from inProgress
// - if key is in dirty, remove it from there and add it to the back of the jobQueue
func storeResult(d *deployer, key string, err error, logger logr.Logger) {
	d.mu.Lock()

	// Remove from inProgress
	for i := range d.inProgress {
		if d.inProgress[i] != key {
			continue
		}
		logger.V(logs.LogDebug).Info("remove from inProgress")
		d.inProgress = removeFromSlice(d.inProgress, i)
		break
	}

	l := logger.WithValues("key", key)

	if err != nil {
		l.V(logs.LogInfo).Info(fmt.Sprintf("added to result with err %s", err.Error()))
	} else {
		l.V(logs.LogInfo).Info("added to result")
	}
	d.results[key] = err

	// if key is in dirty, remove from there and push to jobQueue
	for i := range d.dirty {
		if d.dirty[i] != key {
			continue
		}
		l.V(logs.LogDebug).Info("add to jobQueue")
		d.jobQueue = append(d.jobQueue, requestParams{key: d.dirty[i]})
		l.V(logs.LogDebug).Info("remove from dirty")
		d.dirty = removeFromSlice(d.dirty, i)
		l.V(logs.LogDebug).Info("remove result")
		delete(d.results, key)
		break
	}

	d.mu.Unlock()
}

// getRequestStatus gets requests status.
// If result is available it returns the result.
// If request is still queued, responseParams is nil and an error is nil.
// If result is not available and request is neither queued nor already processed, it returns an error to indicate that.
func getRequestStatus(d *deployer, snapshostName string) (*responseParams, error) {
	logger := d.log.WithValues("key", snapshostName)

	d.mu.Lock()
	defer d.mu.Unlock()

	logger.V(logs.LogDebug).Info("searching result")
	if _, ok := d.results[snapshostName]; ok {
		logger.V(logs.LogDebug).Info("request already processed, result present. returning result.")
		if d.results[snapshostName] != nil {
			logger.V(logs.LogDebug).Info("returning a response with an error")
		}
		resp := responseParams{
			requestParams: requestParams{
				key: snapshostName,
			},
			err: d.results[snapshostName],
		}
		logger.V(logs.LogDebug).Info("removing result")
		delete(d.results, snapshostName)
		return &resp, nil
	}

	for i := range d.inProgress {
		if d.inProgress[i] == snapshostName {
			logger.V(logs.LogDebug).Info("request is still in inProgress, so being processed")
			return nil, nil
		}
	}

	for i := range d.jobQueue {
		if d.jobQueue[i].key == snapshostName {
			logger.V(logs.LogDebug).Info("request is still in jobQueue, so waiting to be processed.")
			return nil, nil
		}
	}

	// if we get here it means, we have no response for this workload cluster, nor the
	// request is queued or being processed
	logger.V(logs.LogDebug).Info("request has not been processed nor is currently queued.")
	return nil, fmt.Errorf("request has not been processed nor is currently queued")
}

func removeFromSlice(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func collectSnapshot(ctx context.Context, c client.Client, snapshotName string, logger logr.Logger) error {
	// Get Snapshot instance
	snapshotInstance := &utilsv1alpha1.Snapshot{}
	err := c.Get(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Snapshot %s does not exist anymore. Nothing to do.", snapshotName))
		}
	}

	err = cleanOldSnapshots(snapshotInstance, logger)
	if err != nil {
		return err
	}

	// All snapshots for a given Snapshot instance are contained in the same directory
	currentTime := time.Now()
	artifactFolder := getArtifactFolderName(snapshotInstance)
	folder := filepath.Join(artifactFolder, currentTime.Format(timeFormat))
	// Collect all ClusterProfiles
	logger.V(logs.LogDebug).Info(fmt.Sprintf("snapshot will stored in path is %s", artifactFolder))
	err = dumpClusterProfiles(ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClusterConfigurations(ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClusters(ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClassifiers(ctx, folder, logger)
	if err != nil {
		return err
	}
	return nil
}

func dumpClassifiers(ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing Classifiers")
	classifiers, err := utils.GetAccessInstance().ListClassifiers(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Classifiers", len(classifiers.Items)))
	for i := range classifiers.Items {
		cl := &classifiers.Items[i]
		err = DumpObject(cl, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpClusterProfiles(ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing ClusterProfiles")
	clusterProfiles, err := utils.GetAccessInstance().ListClusterProfiles(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d ClusterProfiles", len(clusterProfiles.Items)))
	for i := range clusterProfiles.Items {
		cc := &clusterProfiles.Items[i]
		err = DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
		err = dumpReferencedObjects(ctx, cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpReferencedObjects(ctx context.Context, clusterProfile *configv1alpha1.ClusterProfile,
	folder string, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info("storing ClusterProfiles's referenced resources")
	var object client.Object
	for i := range clusterProfile.Spec.PolicyRefs {
		ref := &clusterProfile.Spec.PolicyRefs[i]
		if ref.Kind == string(configv1alpha1.ConfigMapReferencedResourceKind) {
			configMap := &corev1.ConfigMap{}
			err := utils.GetAccessInstance().GetResource(ctx,
				types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, configMap)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Found referenced ConfigMap %s/%s", configMap.Namespace, configMap.Name))
			object = configMap
		} else {
			// TODO: Allow certain Secret to be skipped
			secret := &corev1.Secret{}
			err := utils.GetAccessInstance().GetResource(ctx,
				types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, secret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Found referenced Secret %s/%s", secret.Namespace, secret.Name))
			object = secret
		}

		if err := DumpObject(object, folder, logger); err != nil {
			return err
		}
	}

	return nil
}

func dumpClusterConfigurations(ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing ClusterConfigurations")
	clusterConfigurations, err := utils.GetAccessInstance().ListClusterConfigurations(ctx, "", logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d ClusterConfigurations", len(clusterConfigurations.Items)))
	for i := range clusterConfigurations.Items {
		cc := &clusterConfigurations.Items[i]
		err = DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpCAPIClusters(ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing CAPI Clusters")
	clusterList := &clusterv1.ClusterList{}
	err := utils.GetAccessInstance().ListResources(ctx, clusterList)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Clusters", len(clusterList.Items)))
	for i := range clusterList.Items {
		cc := &clusterList.Items[i]
		err = DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpSveltosClusters(ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing Sveltos Clusters")
	clusterList := &libsveltosv1alpha1.SveltosClusterList{}
	err := utils.GetAccessInstance().ListResources(ctx, clusterList)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Clusters", len(clusterList.Items)))
	for i := range clusterList.Items {
		cc := &clusterList.Items[i]
		err = DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpClusters(ctx context.Context, folder string, logger logr.Logger) error {
	if err := dumpCAPIClusters(ctx, folder, logger); err != nil {
		return err
	}

	if err := dumpSveltosClusters(ctx, folder, logger); err != nil {
		return err
	}

	return nil
}

// DumpObject is a helper function to generically dump resource definition
// given the resource reference and file path for dumping location.
func DumpObject(resource client.Object, logPath string, logger logr.Logger) error {
	// Do not store resource version
	resource.SetResourceVersion("")
	err := addTypeInformationToObject(resource)
	if err != nil {
		return err
	}

	resourceYAML, err := yaml.Marshal(resource)
	if err != nil {
		return err
	}

	metaObj, err := apimeta.Accessor(resource)
	if err != nil {
		return err
	}

	kind := resource.GetObjectKind().GroupVersionKind().Kind
	namespace := metaObj.GetNamespace()
	name := metaObj.GetName()

	resourceFilePath := path.Join(logPath, namespace, kind, name+".yaml")
	err = os.MkdirAll(filepath.Dir(resourceFilePath), permission0755)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(resourceFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, permission0644)
	if err != nil {
		return err
	}
	defer f.Close()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("storing %s %s/%s in %s", kind, namespace, name, resourceFilePath))
	return os.WriteFile(f.Name(), resourceYAML, permission0600)
}

func cleanOldSnapshots(snapshotInstance *utilsv1alpha1.Snapshot, logger logr.Logger) error {
	if snapshotInstance.Spec.SuccessfulSnapshotLimit == nil {
		return nil
	}

	results, err := listCollectionsForSnapshot(snapshotInstance, logger)
	if err != nil {
		return err
	}

	timeSlice := make([]time.Time, 0)

	for i := range results {
		t, err := time.Parse(timeFormat, results[i])
		if err != nil {
			continue
		}
		timeSlice = append(timeSlice, t)
	}

	sort.Slice(timeSlice, func(i, j int) bool {
		return timeSlice[i].Before(timeSlice[j])
	})

	// Remove oldest directories
	artifactFolder := getArtifactFolderName(snapshotInstance)
	for i := 0; i < len(timeSlice)-int(*snapshotInstance.Spec.SuccessfulSnapshotLimit); i++ {
		dirName := timeSlice[i].Format(timeFormat)
		err := os.RemoveAll(filepath.Join(artifactFolder, dirName))
		if err != nil {
			return err
		}
	}
	return nil
}

func listCollectionsForSnapshot(snapshotInstance *utilsv1alpha1.Snapshot, logger logr.Logger,
) ([]string, error) {

	artifactFolder := getArtifactFolderName(snapshotInstance)

	logger.V(logs.LogDebug).Info(fmt.Sprintf("getting content for directory: %s", artifactFolder))

	files, err := os.ReadDir(artifactFolder)
	if err != nil {
		return nil, err
	}

	results := make([]string, 0)
	for i := range files {
		if files[i].IsDir() {
			results = append(results, files[i].Name())
		}
	}

	return results, nil
}

func getArtifactFolderName(snapshotInstance *utilsv1alpha1.Snapshot) string {
	return filepath.Join(snapshotInstance.Spec.Storage, snapshotInstance.Name)
}

func addTypeInformationToObject(obj client.Object) error {
	scheme, err := utils.GetScheme()
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
