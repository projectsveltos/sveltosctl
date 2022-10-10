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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
)

const (
	unavailable = "unavailable"
)

type ResultStatus int64

const (
	Collected ResultStatus = iota
	InProgress
	Failed
	Unavailable
)

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

type SnapshotInterface interface {
	// Collect creates a request to take a snapshot.
	// When worker is available to fulfill such request, RequestHandler
	// will be invoked in the worker context.
	// snapshotName is the name of the Snaphost instance making this request.
	Collect(ctx context.Context, snapshotName string) error

	// IsInProgress returns true, request to take a snaphost currently in progress.
	IsInProgress(snapshot string) bool

	// GetResult returns result for a given request.
	GetResult(ctx context.Context, snapshostName string) Result

	// ListSnapshots returns list all snapshots taken for a given Snapshot instance
	ListSnapshots(snapshotInstance *utilsv1alpha1.Snapshot,
		logger logr.Logger) ([]string, error)

	// GetCollectedSnapshotFolder returns the artifact folder where snapshots are
	// collected for a given Snapshot instance
	GetCollectedSnapshotFolder(snapshotInstance *utilsv1alpha1.Snapshot,
		logger logr.Logger) (*string, error)

	// GetNamespacedResources returns all namespaced resources contained in the
	// snapshotFoler.
	// Returns a map with:
	// - key: <namespace name>
	// - value: list of resources of the Kind specified
	GetNamespacedResources(snapshotFolder, kind string, logger logr.Logger,
	) (map[string][]*unstructured.Unstructured, error)

	// CleanupEntries removes any entry (from any internal data structure) for
	// given snapshot request
	CleanupEntries(snapshotInstance *utilsv1alpha1.Snapshot) error
}
