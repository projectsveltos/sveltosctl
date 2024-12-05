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

package utils

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

// ListClusterReports returns all current ClusterReports in a namespace (if specified)
func (a *k8sAccess) ListClusterReports(ctx context.Context, namespace string,
	logger logr.Logger) (*configv1beta1.ClusterReportList, error) {

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
	}

	logger.V(logs.LogDebug).Info("Get all ClusterReports")
	clusterReports := &configv1beta1.ClusterReportList{}
	err := a.client.List(ctx, clusterReports, listOptions...)
	return clusterReports, err
}

// SortClusterReports sorts ClusterReports by Cluster Namespace/Name
func (a *k8sAccess) SortClusterReports(clusterReports []configv1beta1.ClusterReport) []configv1beta1.ClusterReport {
	sort.Slice(clusterReports, func(i, j int) bool {
		if clusterReports[i].Spec.ClusterNamespace == clusterReports[j].Spec.ClusterNamespace {
			return clusterReports[i].Spec.ClusterName < clusterReports[j].Spec.ClusterName
		}
		return clusterReports[i].Spec.ClusterNamespace < clusterReports[j].Spec.ClusterNamespace
	})

	return clusterReports
}
