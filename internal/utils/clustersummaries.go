/*
Copyright 2025. projectsveltos.io. All rights reserved.

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
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

// OutdatedHelmChartInfo mirrors the subset of HelmChartSummary reporting the outcome of
// addon-controller's periodic outdated-Helm-chart check for a single helm release.
type OutdatedHelmChartInfo struct {
	LatestVersion      string
	LatestPatchVersion string
}

// GetOutdatedHelmChartInfo lists every ClusterSummary for the cluster identified by
// namespace/clusterName/clusterType and returns a map, keyed by "<releaseNamespace>/<releaseName>",
// of outdated-version info from each actively-managed (Status == Managing) HelmChartSummary entry.
func (a *k8sAccess) GetOutdatedHelmChartInfo(ctx context.Context, namespace, clusterName, clusterType string,
	logger logr.Logger) (map[string]OutdatedHelmChartInfo, error) {

	logger = logger.WithValues("namespace", namespace, "cluster", clusterName, "clusterType", clusterType)
	logger.V(logs.LogDebug).Info("Get ClusterSummaries for outdated helm chart info")

	clusterSummaries := &configv1beta1.ClusterSummaryList{}
	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			configv1beta1.ClusterNameLabel: clusterName,
			configv1beta1.ClusterTypeLabel: clusterType,
		},
	}

	if err := a.client.List(ctx, clusterSummaries, listOptions...); err != nil {
		return nil, err
	}

	result := map[string]OutdatedHelmChartInfo{}
	for i := range clusterSummaries.Items {
		cs := &clusterSummaries.Items[i]
		for j := range cs.Status.HelmReleaseSummaries {
			rs := &cs.Status.HelmReleaseSummaries[j]
			if rs.Status != configv1beta1.HelmChartStatusManaging {
				continue
			}

			if rs.LatestVersion == nil && rs.LatestPatchVersion == nil {
				continue
			}

			info := OutdatedHelmChartInfo{}
			if rs.LatestVersion != nil {
				info.LatestVersion = *rs.LatestVersion
			}
			if rs.LatestPatchVersion != nil {
				info.LatestPatchVersion = *rs.LatestPatchVersion
			}
			result[HelmReleaseKey(rs.ReleaseNamespace, rs.ReleaseName)] = info
		}
	}

	return result, nil
}

// HelmReleaseKey returns the join key used to match a deployed Chart (from ClusterConfiguration)
// to its OutdatedHelmChartInfo (from ClusterSummary).
func HelmReleaseKey(releaseNamespace, releaseName string) string {
	return fmt.Sprintf("%s/%s", releaseNamespace, releaseName)
}
