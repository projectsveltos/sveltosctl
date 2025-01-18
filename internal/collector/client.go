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

package collector

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2/textlogger"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

var (
	getClientLock     = &sync.Mutex{}
	collectorInstance *Collector
)

const (
	tarExtension = ".tar.gz"
)

// Collector represents a client implementing the CollectorInterface
type Collector struct {
	log logr.Logger
	client.Client

	mu *sync.Mutex

	// A request represents a request to collect resources/logs (for instance
	// a snapshot request).

	// dirty contains all requests which are currently waiting to be served.
	dirty []string

	// inProgress contains all request that are currently being served.
	inProgress []string

	// jobQueue contains all requests that needs to be served.
	jobQueue []requestParams

	// results contains results for processed requests
	results map[string]error
}

// InitializeClient initializes a client implementing the CollectorInterface
func InitializeClient(ctx context.Context, l logr.Logger, c client.Client, numOfWorker int) {
	if collectorInstance == nil {
		getClientLock.Lock()
		defer getClientLock.Unlock()
		if collectorInstance == nil {
			l.V(logs.LogInfo).Info(fmt.Sprintf("Creating instance now. Number of workers: %d", numOfWorker))
			collectorInstance = &Collector{log: l, Client: c}
			collectorInstance.log = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
			collectorInstance.startWorkloadWorkers(ctx, numOfWorker, l)
		}
	}
}

// GetClient return a collector client, implementing the CollectorInterface
func GetClient() *Collector {
	getClientLock.Lock()
	defer getClientLock.Unlock()
	return collectorInstance
}

func (d *Collector) Collect(ctx context.Context, requestorName string,
	collectionType CollectionType, collectMethd CollectMethod) error {

	d.mu.Lock()
	defer d.mu.Unlock()

	l := d.log.WithValues("requestor", requestorName, "type", collectionType.string())
	key := getKey(requestorName, collectionType)

	// Search if request is in dirty. Drop it if already there
	for i := range d.dirty {
		if d.dirty[i] == key {
			l.V(logs.LogDebug).Info("request is already present in dirty")
			return nil
		}
	}

	// Since we got a new request, if a result was saved, clear it.
	l.V(logs.LogDebug).Info("removing result from previous request if any")
	delete(d.results, key)

	d.log.V(logs.LogDebug).Info("request added to dirty")
	d.dirty = append(d.dirty, key)

	// Push to queue if not already in progress
	for i := range d.inProgress {
		if d.inProgress[i] == key {
			d.log.V(logs.LogDebug).Info("request is already in inProgress")
			return nil
		}
	}

	d.log.V(logs.LogDebug).Info("request added to jobQueue")
	req := requestParams{requestorName: requestorName,
		collectionType: collectionType,
		collectMethod:  collectMethd,
	}
	d.jobQueue = append(d.jobQueue, req)

	return nil
}

func (d *Collector) GetResult(ctx context.Context, requestorName string,
	collectionType CollectionType) Result {

	responseParam, err := getRequestStatus(d, requestorName, collectionType)
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

func (d *Collector) ListCollections(storage, requestorName string, collectionType CollectionType,
	logger logr.Logger) ([]string, error) {

	return listCollectionsForRequestor(storage, requestorName, collectionType, logger)
}

func (d *Collector) GetFolder(storage, requestorName string, collectionType CollectionType,
	logger logr.Logger) (*string, error) {

	l := logger.WithValues("requestor", requestorName, "type", collectionType.string())
	l.V(logs.LogDebug).Info("getting directory containing collections for instance")

	artifactFolder := getArtifactFolderName(storage, requestorName, collectionType)

	if _, err := os.Stat(artifactFolder); os.IsNotExist(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("directory %s not found", artifactFolder))
		return nil, err
	}

	return &artifactFolder, nil
}

func (d *Collector) TarDir(src string, logger logr.Logger) error {
	logger = logger.WithValues("folder", src)
	logger.V(logs.LogDebug).Info("compress directory")
	var buf bytes.Buffer
	if err := compress(src, &buf, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("compress failed with error :%v", err))
		return err
	}

	base := filepath.Base(src)
	output := filepath.Join(src, base+tarExtension)

	err := os.MkdirAll(filepath.Dir(output), permission0755)
	if err != nil {
		return err
	}

	fileToWrite, err := os.OpenFile(output, os.O_CREATE|os.O_RDWR, os.FileMode(permission0600))
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("compress failed with error :%v", err))
		return err
	}
	if _, err := io.Copy(fileToWrite, &buf); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("compress failed with error :%v", err))
		return err
	}
	return nil
}

func (d *Collector) GetFolderPath(storage, requestorName string, collectionType CollectionType, t time.Time) string {
	artifactFolder := getArtifactFolderName(storage, requestorName, collectionType)
	timeFolder := t.Format(timeFormat)
	return filepath.Join(artifactFolder, timeFolder)
}

func (d *Collector) GetNamespacedResources(folder, kind string, logger logr.Logger,
) (map[string][]*unstructured.Unstructured, error) {

	fileInfo, err := getFileInfo(folder, logger)
	if err != nil {
		return nil, err
	}

	if !fileInfo.IsDir() {
		msg := fmt.Sprintf("file %s is not a collection directory", folder)
		logger.V(logs.LogDebug).Info(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	files, err := os.ReadDir(folder)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to list subdirectories in %s. Err: %v",
			folder, err))
		return nil, err
	}

	// Looking for Namespaced resources. So walk all subdirectory (each subdirectory can represent a namespace)
	// and collect resources of the specified Kind in each subdirectory
	result := make(map[string][]*unstructured.Unstructured)
	for i := range files {
		if files[i].IsDir() {
			namespaceDirectory := filepath.Join(folder, files[i].Name())
			r, err := d.getResourcesForKind(namespaceDirectory, kind, logger)
			if err != nil {
				return nil, err
			}
			if len(r) > 0 {
				logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d resources of kind %s in namespace %s (folder %s)",
					len(r), kind, files[i].Name(), namespaceDirectory))
				result[files[i].Name()] = r
			}
		}
	}

	return result, nil
}

func (d *Collector) GetClusterResources(folder, kind string, logger logr.Logger,
) ([]*unstructured.Unstructured, error) {

	fileInfo, err := getFileInfo(folder, logger)
	if err != nil {
		return nil, err
	}

	if !fileInfo.IsDir() {
		msg := fmt.Sprintf("file %s is not a collection directory", folder)
		logger.V(logs.LogDebug).Info(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	return d.getResourcesForKind(folder, kind, logger)
}

func (d *Collector) getResourcesForKind(directory, kind string, logger logr.Logger) ([]*unstructured.Unstructured, error) {
	// Each directory, contains one subdirectory per Kind
	// For instance /<whatever>/<snapshotInstanceName>/<dateSnaphostTaken>/<namespaceName>/<kindName>
	// within such directory there all resources of that type found at the time snapshot was taken

	kindPath := filepath.Join(directory, kind)
	logger.V(logs.LogDebug).Info(fmt.Sprintf("find resource of kind %s in folder %s", kind, kindPath))

	if _, err := os.Stat(kindPath); os.IsNotExist(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Subdirectory %s contains no resource of kind %s",
			directory, kind))
		return nil, nil
	}

	files, err := os.ReadDir(kindPath)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to list subdirectories in %s. Err: %v",
			kindPath, err))
		return nil, err
	}

	result := make([]*unstructured.Unstructured, 0)
	for i := range files {
		if files[i].IsDir() {
			continue
		}
		logger.V(logs.LogDebug).Info(fmt.Sprintf("collecting %s resources in directory %s",
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
func (d *Collector) GetUnstructured(object []byte) (*unstructured.Unstructured, error) {
	request := &unstructured.Unstructured{}
	universalDeserializer := scheme.Codecs.UniversalDeserializer()
	_, _, err := universalDeserializer.Decode(object, nil, request)
	if err != nil {
		return nil, fmt.Errorf("failed to decode k8s resource %s: %w",
			string(object), err)
	}

	return request, nil
}

func (d *Collector) IsInProgress(requestorName string, collectionType CollectionType) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := getKey(requestorName, collectionType)

	for i := range d.inProgress {
		if d.inProgress[i] == key {
			d.log.V(logs.LogDebug).Info("request is already in inProgress")
			return true
		}
	}

	return false
}

func (d *Collector) CleanupEntries(storage, requestorName string, collectionType CollectionType) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := getKey(requestorName, collectionType)

	for i := range d.dirty {
		if d.dirty[i] != key {
			continue
		}
		d.dirty = append(d.dirty[:i], d.dirty[i+1:]...)
	}

	for i := range d.jobQueue {
		if d.jobQueue[i].requestorName != requestorName &&
			d.jobQueue[i].collectionType == collectionType {

			continue
		}
		d.jobQueue = append(d.jobQueue[:i], d.jobQueue[i+1:]...)
	}

	delete(d.results, key)

	artifactFolder := getArtifactFolderName(storage, requestorName, collectionType)
	return os.RemoveAll(artifactFolder)
}

func (d *Collector) CleanOldCollections(storage, requestorName string, collectionType CollectionType,
	limit int32, logger logr.Logger) error {

	return cleanOldCollections(storage, requestorName, collectionType, limit, logger)
}

// DumpObject is a helper function to generically dump resource definition
// given the resource reference and file path for dumping location.
func (d *Collector) DumpObject(resource client.Object, logPath string, logger logr.Logger) error {
	// Do not store resource version
	resource.SetResourceVersion("")
	err := addTypeInformationToObject(resource)
	if err != nil {
		return err
	}

	logger = logger.WithValues("kind", resource.GetObjectKind())
	logger = logger.WithValues("resource", fmt.Sprintf("%s %s",
		resource.GetNamespace(), resource.GetName()))

	if !resource.GetDeletionTimestamp().IsZero() {
		logger.V(logs.LogDebug).Info("resource is marked for deletion. Do not collect it.")
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

	logger.V(logs.LogDebug).Info(fmt.Sprintf("storing resource in %s", resourceFilePath))
	return os.WriteFile(f.Name(), resourceYAML, permission0600)
}

// DumpPodLogs collects logs for all containers in a pod and store them.
// If pod has restarted, it will try to collect log from previous run as well.
func (d *Collector) DumpPodLogs(ctx context.Context, clientSet *kubernetes.Clientset, logPath string,
	since *int64, pod *corev1.Pod) error {

	for i := range pod.Spec.Containers {
		container := &pod.Spec.Containers[i]
		resourceFilePath := path.Join(logPath, "logs", pod.Namespace, pod.Name+"-"+container.Name)
		err := os.MkdirAll(filepath.Dir(resourceFilePath), permission0755)
		if err != nil {
			return err
		}

		err = collectLogs(ctx, clientSet, pod.Namespace, pod.Name, container.Name, resourceFilePath, since, false)
		if err != nil {
			return err
		}

		// If container restarted, collect previous logs as well
		for i := range pod.Status.ContainerStatuses {
			containerStatus := &pod.Status.ContainerStatuses[i]
			if containerStatus.Name == container.Name &&
				containerStatus.RestartCount > 0 {

				resourceFilePath := path.Join(logPath, "logs", pod.Namespace,
					pod.Name+"-"+container.Name+".previous")

				err := os.MkdirAll(filepath.Dir(resourceFilePath), permission0755)
				if err != nil {
					return err
				}

				err = collectLogs(ctx, clientSet, pod.Namespace, pod.Name, container.Name, resourceFilePath, since, true)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// collectLogs collect logs for a given namespace/pod container
func collectLogs(ctx context.Context, clientset *kubernetes.Clientset,
	namespace, podName, containerName, filename string, since *int64, previous bool) (err error) {
	// open output file
	var fo *os.File
	fo, err = os.Create(filename)
	if err != nil {
		return err
	}
	// close fo on exit and check for its returned error
	defer func() {
		if cerr := fo.Close(); cerr != nil {
			if err == nil {
				err = cerr
			}
		}
	}()

	podLogOpts := corev1.PodLogOptions{}
	if containerName != "" {
		podLogOpts.Container = containerName
	}

	if previous {
		podLogOpts.Previous = previous
	}

	if since != nil {
		podLogOpts.SinceSeconds = since
	}

	req := clientset.CoreV1().Pods(namespace).GetLogs(podName, &podLogOpts)
	var podLogs io.ReadCloser
	podLogs, err = req.Stream(ctx)
	if err != nil {
		return err
	}
	defer podLogs.Close()

	_, err = io.Copy(fo, podLogs)

	return err
}

func getFileInfo(folder string, logger logr.Logger) (fs.FileInfo, error) {
	file, err := os.Open(folder)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to open directory %s. Err: %v",
			folder, err))
		return nil, err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to get FileInfo for %s. Err: %v",
			folder, err))
		return nil, err
	}

	return fileInfo, nil
}

// startWorkloadWorkers initializes all internal structures and starts
// pool of workers
// - numWorker is number of requested workers
// - c is the kubernetes client to access control cluster
func (d *Collector) startWorkloadWorkers(ctx context.Context, numOfWorker int, logger logr.Logger) {
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

func getKey(requestorName string, collectionType CollectionType) string {
	return fmt.Sprintf("%s:%s", collectionType.string(), requestorName)
}

// compress takes a source and walks 'source' writing each file to buf.
func compress(src string, buf io.Writer, logger logr.Logger) error {
	// ensure the src actually exists before trying to tar it
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("unable to tar files - %w", err)
	}

	// tar > gzip > buf
	zr := gzip.NewWriter(buf)
	tw := tar.NewWriter(zr)

	// walk through every file in the folder
	err := filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {
		// generate tar header
		header, inErr := tar.FileInfoHeader(fi, file)
		if inErr != nil {
			return inErr
		}

		// must provide real name
		// (see https://golang.org/src/archive/tar/common.go?#L626)
		header.Name = filepath.ToSlash(file)

		// write header
		if inErr := tw.WriteHeader(header); inErr != nil {
			return inErr
		}
		// if not a dir, write file content
		if !fi.IsDir() {
			data, inErr := os.Open(file)
			if inErr != nil {
				return inErr
			}
			if _, inErr := io.Copy(tw, data); inErr != nil {
				return inErr
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	// produce tar
	if err := tw.Close(); err != nil {
		return err
	}
	// produce gzip
	if err := zr.Close(); err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("removing old content")
	if err := removeContents(src); err != nil {
		return err
	}

	return nil
}

func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}

	for _, name := range names {
		if strings.Contains(name, tarExtension) {
			// Do not remove the tar.gz file
			continue
		}
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}
