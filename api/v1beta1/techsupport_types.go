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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

const (
	// TechsupportFinalizer allows TechsupportReconciler to clean up resources associated with
	// Techsupport instance before removing it from the apiserver.
	TechsupportFinalizer = "techsupportfinalizer.projectsveltos.io"
)

// Resource indicates the type of resources to collect.
type Resource struct {
	// Namespace of the resource deployed in the Cluster.
	// Empty for resources scoped at cluster level.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Group of the resource deployed in the Cluster.
	Group string `json:"group"`

	// Version of the resource deployed in the Cluster.
	Version string `json:"version"`

	// Kind of the resource deployed in the Cluster.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// LabelFilters allows to filter resources based on current labels.
	LabelFilters []libsveltosv1beta1.LabelFilter `json:"labelFilters,omitempty"`
}

// LogFilter allows to select which logs to collect
type Log struct {
	// Namespace of the pods deployed in the Cluster.
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// LabelFilters allows to filter pods based on current labels.
	LabelFilters []libsveltosv1beta1.LabelFilter `json:"labelFilters,omitempty"`

	// A relative time in seconds before the current time from which to collect logs.
	// If this value precedes the time a pod was started, only logs since the pod start will be returned.
	// If this value is in the future, no logs will be returned. Only one of sinceSeconds or sinceTime may be specified.
	// +optional
	SinceSeconds *int64 `json:"sinceSeconds,omitempty"`
}

// TechsupportSpec defines the desired state of Techsupport
type TechsupportSpec struct {
	// ClusterSelector identifies clusters to collect techsupport from.
	ClusterSelector libsveltosv1beta1.Selector `json:"clusterSelector"`

	// Resources indicates what resorces to collect
	// +optional
	Resources []Resource `json:"resources,omitempty"`

	// Logs indicates what pods' log to collect
	// +optional
	Logs []Log `json:"logs,omitempty"`

	// If set denerates a tar file with all collected logs/resources
	// +kubebuilder:default:=false
	// +optional
	Tar bool `json:"tar,omitempty"`

	// Schedule in Cron format, see https://en.wikipedia.org/wiki/Cron.
	Schedule string `json:"schedule"`

	// Optional deadline in seconds for starting the job if it misses scheduled
	// time for any reason.  Missed jobs executions will be counted as failed ones.
	// +optional
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	// Storage represents directory where techsupports will be stored.
	// It must be an existing directory.
	// Techsupports will be stored in this directory in a subdirectory named
	// with Techsupport instance name.
	Storage string `json:"storage"`

	// The number of successful finished techsupport to retains.
	// If specified, only SuccessfulTechsupportLimit will be retained. Once such
	// number is reached, for any new successful snapshots, the oldest one is
	// deleted.
	// +optional
	SuccessfulTechsupportLimit *int32 `json:"successfulTechsupportLimit,omitempty"`
}

// TechsupportStatus defines the observed state of Techsupport
type TechsupportStatus struct {
	// Information when next snapshot is scheduled
	// +optional
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// Information when was the last time a snapshot was successfully scheduled.
	// +optional
	LastRunTime *metav1.Time `json:"lastRunTime,omitempty"`

	// Status indicates what happened to last techsupport collection.
	LastRunStatus *CollectionStatus `json:"lastRunStatus,omitempty"`

	// FailureMessage provides more information about the error, if
	// any occurred
	FailureMessage *string `json:"failureMessage,omitempty"`

	// MatchingClusterRefs reference all the clusters currently matching
	// Techsupport
	MatchingClusterRefs []corev1.ObjectReference `json:"machingClusters,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:path=techsupports,scope=Cluster
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// Techsupport is the Schema for the snapshot API
type Techsupport struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TechsupportSpec   `json:"spec,omitempty"`
	Status TechsupportStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// TechsupportList contains a list of Techsupport instances
type TechsupportList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Techsupport `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Techsupport{}, &TechsupportList{})
}
