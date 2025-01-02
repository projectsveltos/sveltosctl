/*
Copyright 2023. projectsveltos.io. All rights reserved.

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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// A "request" represents the need to collect resources (for instance a snapshot request).
//
// The flow is following:
// - when a request arrives, it is first added to the dirty set or dropped if it already
// present in the dirty set;
// - pushed to the jobQueue only if it is not presented in inProgress (we don't want
// to process same request in parallel)
//
// When a worker is ready to serve a request, it gets the request from the
// front of the jobQueue.
// The request is also added to the inProgress set and removed from the dirty set.
//
// If a request, currently in the inProgress arrives again, such request is only added
// to the dirty set, not to the queue. This guarantees that same request to collect resources
// is never process more than once in parallel.
//
// When worker is done, the request is removed from the inProgress set.
// If the same request is also present in the dirty set, it is added back to the back of the jobQueue.

type requestParams struct {
	requestorName  string
	collectionType CollectionType
	collectMethod  CollectMethod
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

func processRequests(ctx context.Context, collector *Collector, i int, logger logr.Logger) {
	id := i
	var params *requestParams

	logger.V(logs.LogDebug).Info(fmt.Sprintf("started worker %d", id))

	for {
		if params != nil {
			l := logger.WithValues("requestor", params.requestorName)
			// Get error only from getIsCleanupFromKey as same key is always used
			l.Info(fmt.Sprintf("worker: %d processing request for %s:%s", id,
				params.collectionType.string(), params.requestorName))
			err := params.collectMethod(ctx, controlClusterClient, params.requestorName, l)
			storeResult(collector, params.requestorName, params.collectionType, params.collectMethod, err, l)
		}
		params = nil
		select {
		case <-time.After(1 * time.Second):
			collector.mu.Lock()
			if len(collector.jobQueue) > 0 {
				// take a request from queue and remove it from queue
				params = &requestParams{
					requestorName:  collector.jobQueue[0].requestorName,
					collectionType: collector.jobQueue[0].collectionType,
					collectMethod:  collector.jobQueue[0].collectMethod}
				collector.jobQueue = collector.jobQueue[1:]
				l := logger.WithValues("requestor", params.requestorName)
				l.V(logs.LogDebug).Info("take from jobQueue")
				// Add to inProgress
				l.V(logs.LogDebug).Info("add to inProgress")
				collector.inProgress = append(collector.inProgress, params.requestorName)
				key := getKey(params.requestorName, params.collectionType)
				// If present remove from dirty
				for i := range collector.dirty {
					if collector.dirty[i] == key {
						l.V(logs.LogDebug).Info("remove from dirty")
						collector.dirty = removeFromSlice(collector.dirty, i)
						break
					}
				}
			}
			collector.mu.Unlock()
		case <-ctx.Done():
			logger.V(logs.LogDebug).Info("context canceled")
			return
		}
	}
}

// doneProcessing does following:
// - set results for further in time lookup
// - remove requestorName from inProgress
// - if key is in dirty, remove it from there and add it to the back of the jobQueue
func storeResult(collector *Collector, requestorName string, collectionType CollectionType,
	collectMethod CollectMethod, err error, logger logr.Logger) {

	collector.mu.Lock()

	// Remove from inProgress
	for i := range collector.inProgress {
		if collector.inProgress[i] != requestorName {
			continue
		}
		logger.V(logs.LogDebug).Info("remove from inProgress")
		collector.inProgress = removeFromSlice(collector.inProgress, i)
		break
	}

	l := logger.WithValues("requestor", requestorName)

	if err != nil {
		l.V(logs.LogInfo).Info(fmt.Sprintf("added to result with err %s", err.Error()))
	} else {
		l.V(logs.LogInfo).Info("added to result")
	}
	collector.results[requestorName] = err

	key := getKey(requestorName, collectionType)
	// if key is in dirty, remove from there and push to jobQueue
	for i := range collector.dirty {
		if collector.dirty[i] != key {
			continue
		}
		l.V(logs.LogDebug).Info("add to jobQueue")
		collector.jobQueue = append(collector.jobQueue,
			requestParams{
				requestorName:  requestorName,
				collectionType: collectionType,
				collectMethod:  collectMethod})
		l.V(logs.LogDebug).Info("remove from dirty")
		collector.dirty = removeFromSlice(collector.dirty, i)
		l.V(logs.LogDebug).Info("remove result")
		delete(collector.results, requestorName)
		break
	}

	collector.mu.Unlock()
}

// getRequestStatus gets requests status.
// If result is available it returns the result.
// If request is still queued, responseParams is nil and an error is nil.
// If result is not available and request is neither queued nor already processed, it returns an error to indicate that.
func getRequestStatus(collector *Collector, requestorName string, collectionType CollectionType,
) (*responseParams, error) {

	logger := collector.log.WithValues("requestor", requestorName, "type", collectionType.string())
	collector.mu.Lock()
	defer collector.mu.Unlock()

	key := getKey(requestorName, collectionType)

	logger.V(logs.LogDebug).Info("searching result")
	if _, ok := collector.results[key]; ok {
		logger.V(logs.LogDebug).Info("request already processed, result present. returning result.")
		if collector.results[key] != nil {
			logger.V(logs.LogDebug).Info("returning a response with an error")
		}
		resp := responseParams{
			requestParams: requestParams{
				requestorName:  requestorName,
				collectionType: collectionType,
			},
			err: collector.results[key],
		}
		logger.V(logs.LogDebug).Info("removing result")
		delete(collector.results, key)
		return &resp, nil
	}

	for i := range collector.inProgress {
		if collector.inProgress[i] == key {
			logger.V(logs.LogDebug).Info("request is still in inProgress, so being processed")
			return nil, nil
		}
	}

	for i := range collector.jobQueue {
		if collector.jobQueue[i].requestorName == requestorName &&
			collector.jobQueue[i].collectionType == collectionType {

			logger.V(logs.LogDebug).Info("request is still in jobQueue, so waiting to be processed.")
			return nil, nil
		}
	}

	// if we get here it means, we have no response for this requestorName, nor the
	// request is queued or being processed
	logger.V(logs.LogDebug).Info("request has not been processed nor is currently queued.")
	return nil, fmt.Errorf("request has not been processed nor is currently queued")
}

func removeFromSlice(s []string, i int) []string {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
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

func getArtifactFolderName(storage, requestorName string, collectionType CollectionType) string {
	return filepath.Join(storage, collectionType.string(), requestorName)
}

func cleanOldCollections(storage, requestorName string, collectionType CollectionType,
	limit int32, logger logr.Logger) error {

	results, err := listCollectionsForRequestor(storage, requestorName, collectionType, logger)
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
	artifactFolder := getArtifactFolderName(storage, requestorName, collectionType)
	for i := 0; i < len(timeSlice)-int(limit); i++ {
		dirName := timeSlice[i].Format(timeFormat)
		err := os.RemoveAll(filepath.Join(artifactFolder, dirName))
		if err != nil {
			return err
		}
	}
	return nil
}

func listCollectionsForRequestor(storage, requestorName string, collectionType CollectionType,
	logger logr.Logger) ([]string, error) {

	artifactFolder := getArtifactFolderName(storage, requestorName, collectionType)

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
