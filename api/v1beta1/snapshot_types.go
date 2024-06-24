/*
Copyright 2024. projectsveltos.io. All rights reserved.

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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SnapshotFinalizer allows SnapshotReconciler to clean up resources associated with
	// Snapshot instance before removing it from the apiserver.
	SnapshotFinalizer = "snapshotfinalizer.projectsveltos.io"
)

// SnapshotSpec defines the desired state of Snapshot
type SnapshotSpec struct {
	// Schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"schedule"`

	// Optional deadline in seconds for starting the job if it misses scheduled
	// time for any reason.  Missed jobs executions will be counted as failed ones.
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	// Storage represents directory where snapshots will be stored.
	// It must be an existing directory.
	// Snapshots will be stored in this directory in a subdirectory named
	// with Snapshot instance name.
	Storage string `json:"storage"`

	// The number of successful finished snapshots to retains.
	// If specified, only SuccessfulSnapshotLimit will be retained. Once such
	// number is reached, for any new successful snapshots, the oldest one is
	// deleted.
	// +optional
	SuccessfulSnapshotLimit *int32 `json:"successfulSnapshotLimit,omitempty"`
}

// SnapshotStatus defines the observed state of Snapshot
type SnapshotStatus struct {
	// Information when next snapshot is scheduled
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// Information when was the last time a snapshot was successfully scheduled.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// Status indicates what happened to last snapshot collection.
	LastRunStatus *CollectionStatus `json:"lastRunStatus,omitempty"`

	// FailureMessage provides more information about the error, if
	// any occurred
	FailureMessage *string `json:"failureMessage,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:path=snapshots,scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// Snapshot is the Schema for the snapshot API
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotSpec   `json:"spec,omitempty"`
	Status SnapshotStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SnapshotList contains a list of Snapshot
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Snapshot `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Snapshot{}, &SnapshotList{})
}
