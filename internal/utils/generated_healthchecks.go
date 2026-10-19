/*
Copyright 2026. projectsveltos.io. All rights reserved.

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
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	"github.com/projectsveltos/addon-controller/lib/healthmonitoring"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

const (
	// maxSelectorValues is the number of values (ClusterSummary UIDs) in the label selector of one list request
	maxSelectorValues = 100

	// maxSelectedBatches is the number of such requests above which all the generated
	// HealthChecks are listed at once, instead of with one request for each batch
	maxSelectedBatches = 20
)

// ListGeneratedHealthChecks returns, for the ClusterSummaries with the given UIDs, the name of the
// HealthCheck generated for each of them. HealthChecks are cluster scoped and one is generated for each
// ClusterSummary with health monitoring enabled, so listing all of them would cost more as the number
// of clusters grows: only the ones generated for the given UIDs are listed, sending the UIDs in batches
// to keep the label selector of a request short. When there are so many UIDs that it would take dozens
// of requests, all the generated HealthChecks are listed with one request instead.
// Only the metadata of the HealthChecks is fetched.
func (a *k8sAccess) ListGeneratedHealthChecks(ctx context.Context, summaryUIDs []string,
	logger logr.Logger) (map[types.NamespacedName]string, error) {

	generatedChecks := map[types.NamespacedName]string{}
	if len(summaryUIDs) == 0 {
		return generatedChecks, nil
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Get HealthChecks generated for %d ClusterSummaries", len(summaryUIDs)))

	if len(summaryUIDs) > maxSelectedBatches*maxSelectorValues {
		return a.listAllGeneratedHealthChecks(ctx, summaryUIDs)
	}

	for start := 0; start < len(summaryUIDs); start += maxSelectorValues {
		end := min(start+maxSelectorValues, len(summaryUIDs))

		requirement, err := labels.NewRequirement(configv1beta1.HealthMonitoringClusterSummaryUIDLabel,
			selection.In, summaryUIDs[start:end])
		if err != nil {
			return nil, err
		}

		healthChecks := &metav1.PartialObjectMetadataList{}
		healthChecks.SetGroupVersionKind(libsveltosv1beta1.GroupVersion.WithKind(libsveltosv1beta1.HealthCheckKind))
		err = a.client.List(ctx, healthChecks,
			client.MatchingLabelsSelector{Selector: labels.NewSelector().Add(*requirement)})
		if err != nil {
			return nil, err
		}

		addGeneratedHealthChecks(healthChecks, generatedChecks)
	}

	return generatedChecks, nil
}

// listAllGeneratedHealthChecks lists all the generated HealthChecks and keeps the ones generated
// for the ClusterSummaries with the given UIDs.
func (a *k8sAccess) listAllGeneratedHealthChecks(ctx context.Context,
	summaryUIDs []string) (map[types.NamespacedName]string, error) {

	healthChecks := &metav1.PartialObjectMetadataList{}
	healthChecks.SetGroupVersionKind(libsveltosv1beta1.GroupVersion.WithKind(libsveltosv1beta1.HealthCheckKind))
	err := a.client.List(ctx, healthChecks,
		client.HasLabels{configv1beta1.HealthMonitoringClusterSummaryUIDLabel})
	if err != nil {
		return nil, err
	}

	wanted := make(map[string]bool, len(summaryUIDs))
	for _, uid := range summaryUIDs {
		wanted[uid] = true
	}

	selected := &metav1.PartialObjectMetadataList{}
	for i := range healthChecks.Items {
		if wanted[healthChecks.Items[i].Labels[configv1beta1.HealthMonitoringClusterSummaryUIDLabel]] {
			selected.Items = append(selected.Items, healthChecks.Items[i])
		}
	}

	generatedChecks := map[types.NamespacedName]string{}
	addGeneratedHealthChecks(selected, generatedChecks)
	return generatedChecks, nil
}

// addGeneratedHealthChecks maps to the generated HealthCheck name the ClusterSummary
// each HealthCheck is generated for
func addGeneratedHealthChecks(healthChecks *metav1.PartialObjectMetadataList,
	generatedChecks map[types.NamespacedName]string) {

	for i := range healthChecks.Items {
		owner, ok := healthmonitoring.GeneratedCheckOwner(&healthChecks.Items[i])
		if !ok {
			continue
		}
		generatedChecks[types.NamespacedName{Namespace: owner.Namespace, Name: owner.Name}] =
			healthChecks.Items[i].Name
	}
}
