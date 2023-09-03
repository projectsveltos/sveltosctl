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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	unavailable = "unavailable"
)

type CollectionType int64

const (
	Snapshot CollectionType = iota
)

func (c CollectionType) string() string {
	switch c {
	case Snapshot:
		return "snapshot"
	default:
		panic(1)
	}
}

type ResultStatus int64

const (
	Collected ResultStatus = iota
	InProgress
	Failed
	Unavailable
)

type CollectMethod func(ctx context.Context, c client.Client, requestorName string, logger logr.Logger) error

func (r ResultStatus) String() string {
	switch r {
	case Collected:
		return "collected"
	case InProgress:
		return "in-progress"
	case Failed:
		return "failed"
	case Unavailable:
		return unavailable
	}
	return unavailable
}

type Result struct {
	ResultStatus
	Err error
}

type CollectorInterface interface {
	// RegisterMethod registers a collection method
	RegisterMethod(m CollectMethod)

	// Collect creates a request to collect resources for requestorName
	// When worker is available to fulfill such request, the CollectMethod registered
	// will be invoked in the worker context.
	// requestorName is the name of the instance making this request.
	Collect(ctx context.Context, requestorName string,
		collectionType CollectionType, collectMethd CollectMethod) error

	// IsInProgress returns true if requestorName's request to collect is currently in progress.
	IsInProgress(requestorName string, collectionType CollectionType) bool

	// GetResult returns result for requestorName's request
	GetResult(ctx context.Context, requestorName string, collectionType CollectionType) Result

	// ListCollections returns list all collections taken for a given requestorName
	ListCollections(storage, requestorName string, collectionType CollectionType,
		logger logr.Logger) ([]string, error)

	// GetFolderPath returns the path of folder where resources can be stored
	GetFolderPath(storage, requestorName string, collectionType CollectionType) string

	// GetFolder returns the artifact folder where all collections for a given
	// requestorName are stored
	GetFolder(storage, requestorName string, collectionType CollectionType,
		logger logr.Logger) (*string, error)

	// CleanupEntries removes any entry (from any internal data structure) for
	// given requestorName
	CleanupEntries(storage, requestorName string, collectionType CollectionType) error

	// GetNamespacedResources returns all namespaced resources contained in the
	// folder.
	// Returns a map with:
	// - key: <namespace name>
	// - value: list of resources of the Kind specified
	GetNamespacedResources(folder, kind string, logger logr.Logger) (map[string][]*unstructured.Unstructured, error)

	// GetClusterResources	returns all cluster resources contained in the folder
	GetClusterResources(folder, kind string, logger logr.Logger) ([]*unstructured.Unstructured, error)

	// CleanOldCollections removes old collection for requestorName. If more than limit collections
	// are present, the oldest ones are remove up till there are only limit-1 collections left.
	CleanOldCollections(storage, requestorName string, collectionType CollectionType,
		limit int32, logger logr.Logger) error

	// DumpObject is a helper function to generically dump resource definition
	// given the resource reference and file path for dumping location.
	DumpObject(resource client.Object, logPath string, logger logr.Logger) error
}
