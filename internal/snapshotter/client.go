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
	"path/filepath"
	"sync"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
)

var (
	getClientLock    = &sync.Mutex{}
	deployerInstance *deployer
)

// deployer represents a client implementing the DeployerInterface
type deployer struct {
	log logr.Logger
	client.Client

	mu *sync.Mutex

	// A request represents a request to collect a snapshot.

	// dirty contains all snapshot requests which are currently waiting to
	// be served.
	dirty []string

	// inProgress contains all snapshot request that are currently being served.
	inProgress []string

	// jobQueue contains all snapshot requests that needs to be served.
	jobQueue []requestParams

	// results contains results for processed snapshot request
	results map[string]error
}

// InitializeClient initializes a snapshot client implementing the SnapshotInterface
func InitializeClient(ctx context.Context, l logr.Logger, c client.Client, numOfWorker int) {
	if deployerInstance == nil {
		getClientLock.Lock()
		defer getClientLock.Unlock()
		if deployerInstance == nil {
			l.V(logs.LogInfo).Info(fmt.Sprintf("Creating instance now. Number of workers: %d", numOfWorker))
			deployerInstance = &deployer{log: l, Client: c}
			deployerInstance.startWorkloadWorkers(ctx, numOfWorker, l)
		}
	}
}

// GetClient return a deployer client, implementing the DeployerInterface
func GetClient() *deployer {
	getClientLock.Lock()
	defer getClientLock.Unlock()
	return deployerInstance
}

func (d *deployer) Collect(ctx context.Context, snapshotName string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Search if request is in dirty. Drop it if already there
	for i := range d.dirty {
		if d.dirty[i] == snapshotName {
			d.log.V(logs.LogVerbose).Info("request is already present in dirty")
			return nil
		}
	}

	// Since we got a new request, if a result was saved, clear it.
	d.log.V(logs.LogVerbose).Info("removing result from previous request if any")
	delete(d.results, snapshotName)

	d.log.V(logs.LogVerbose).Info("request added to dirty")
	d.dirty = append(d.dirty, snapshotName)

	// Push to queue if not already in progress
	for i := range d.inProgress {
		if d.inProgress[i] == snapshotName {
			d.log.V(logs.LogVerbose).Info("request is already in inProgress")
			return nil
		}
	}

	d.log.V(logs.LogVerbose).Info("request added to jobQueue")
	req := requestParams{key: snapshotName}
	d.jobQueue = append(d.jobQueue, req)

	return nil
}

func (d *deployer) GetResult(ctx context.Context, snapshotName string) Result {
	responseParam, err := getRequestStatus(d, snapshotName)
	if err != nil {
		return Result{
			ResultStatus: Unavailable,
			Err:          nil,
		}
	}

	if responseParam == nil {
		return Result{
			ResultStatus: InProgress,
			Err:          nil,
		}
	}

	if responseParam.err != nil {
		return Result{
			ResultStatus: Failed,
			Err:          responseParam.err,
		}
	}

	return Result{
		ResultStatus: Collected,
	}
}

func (d *deployer) ListSnapshots(snapshotInstance *utilsv1alpha1.Snapshot,
	logger logr.Logger) ([]string, error) {

	return listCollectionsForSnapshot(snapshotInstance, logger)
}

func (d *deployer) GetCollectedSnapshotFolder(snapshotInstance *utilsv1alpha1.Snapshot,
	logger logr.Logger) (*string, error) {

	logger.V(logs.LogVerbose).Info(
		fmt.Sprintf("getting directory containing collected snapshots for instance: %s", snapshotInstance.Name))

	artifactFolder := getArtifactFolderName(snapshotInstance)

	if _, err := os.Stat(artifactFolder); os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("directory %s not found", artifactFolder))
		return nil, err
	}

	return &artifactFolder, nil
}

func (d *deployer) GetNamespacedResources(snapshotFolder, kind string, logger logr.Logger,
) (map[string][]*unstructured.Unstructured, error) {

	file, err := os.Open(snapshotFolder)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to open directory %s. Err: %v",
			snapshotFolder, err))
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to get FileInfo for %s. Err: %v",
			snapshotFolder, err))
		return nil, err
	}

	if !fileInfo.IsDir() {
		msg := fmt.Sprintf("file %s is not a snapshot directory", snapshotFolder)
		logger.V(logs.LogVerbose).Info(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	files, err := os.ReadDir(snapshotFolder)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to list subdirectories in %s. Err: %v",
			snapshotFolder, err))
		return nil, err
	}

	// Looking for Namespaced resources. So walk all subdirectory (each subdirectory can represent a namespace)
	// and collect resources of the specified Kind in each subdirectory
	result := make(map[string][]*unstructured.Unstructured)
	for i := range files {
		if files[i].IsDir() {
			namespaceDirectory := filepath.Join(snapshotFolder, files[i].Name())
			r, err := d.getResourcesForKind(namespaceDirectory, kind, logger)
			if err != nil {
				return nil, err
			}
			if len(r) > 0 {
				logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d resources of kind %s in namespace %s (folder %s)",
					len(r), kind, files[i].Name(), namespaceDirectory))
				result[files[i].Name()] = r
			}
		}
	}

	return result, nil
}

func (d *deployer) GetClusterResources(snapshotFolder, kind string, logger logr.Logger,
) ([]*unstructured.Unstructured, error) {

	file, err := os.Open(snapshotFolder)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to open directory %s. Err: %v",
			snapshotFolder, err))
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to get FileInfo for %s. Err: %v",
			snapshotFolder, err))
		return nil, err
	}

	if !fileInfo.IsDir() {
		msg := fmt.Sprintf("file %s is not a snapshot directory", snapshotFolder)
		logger.V(logs.LogVerbose).Info(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	return d.getResourcesForKind(snapshotFolder, kind, logger)
}

func (d *deployer) getResourcesForKind(directory, kind string, logger logr.Logger) ([]*unstructured.Unstructured, error) {
	// Each directory, contains one subdirectory per Kind
	// For instance /<whatever>/<snapshotInstanceName>/<dateSnaphostTaken>/<namespaceName>/<kindName>
	// within such directory there all resources of that type found at the time snapshot was taken

	kindPath := filepath.Join(directory, kind)
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("find resource of kind %s in folder %s", kind, kindPath))

	if _, err := os.Stat(kindPath); os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Subdirectory %s contains no resource of kind %s",
			directory, kind))
		return nil, nil
	}

	files, err := os.ReadDir(kindPath)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to list subdirectories in %s. Err: %v",
			kindPath, err))
		return nil, err
	}

	result := make([]*unstructured.Unstructured, 0)
	for i := range files {
		if files[i].IsDir() {
			continue
		}
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("collecting %s resources in directory %s",
			kind, kindPath))
		content, err := os.ReadFile(filepath.Join(kindPath, files[i].Name()))
		if err != nil {
			return nil, err
		}
		u, err := d.GetUnstructured(content)
		if err != nil {
			return nil, err
		}

		result = append(result, u)
	}

	return result, nil
}

// GetUnstructured returns an unstructured given a []bytes containing it
func (d *deployer) GetUnstructured(object []byte) (*unstructured.Unstructured, error) {
	request := &unstructured.Unstructured{}
	universalDeserializer := scheme.Codecs.UniversalDeserializer()
	_, _, err := universalDeserializer.Decode(object, nil, request)
	if err != nil {
		return nil, fmt.Errorf("failed to decode k8s resource %s: %w",
			string(object), err)
	}

	return request, nil
}

func (d *deployer) IsInProgress(snapshotName string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.inProgress {
		if d.inProgress[i] == snapshotName {
			d.log.V(logs.LogVerbose).Info("request is already in inProgress")
			return true
		}
	}

	return false
}

func (d *deployer) CleanupEntries(snapshotInstance *utilsv1alpha1.Snapshot) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.dirty {
		if d.dirty[i] != snapshotInstance.Name {
			continue
		}
		d.dirty = append(d.dirty[:i], d.dirty[i+1:]...)
	}

	for i := range d.jobQueue {
		if d.jobQueue[i].key != snapshotInstance.Name {
			continue
		}
		d.jobQueue = append(d.jobQueue[:i], d.jobQueue[i+1:]...)
	}

	delete(d.results, snapshotInstance.Name)

	artifactFolder := getArtifactFolderName(snapshotInstance)
	return os.RemoveAll(artifactFolder)
}
