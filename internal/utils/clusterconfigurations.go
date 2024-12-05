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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

func (a *k8sAccess) GetClusterNameFromClusterConfiguration(clusterConfiguration *configv1beta1.ClusterConfiguration) string {
	clusterName := clusterConfiguration.Name
	if clusterConfiguration.Labels != nil {
		if v, ok := clusterConfiguration.Labels[configv1beta1.ClusterNameLabel]; ok {
			clusterName = v
		}
	}
	return clusterName
}

// ListClusterConfigurations returns all current ClusterConfigurations in a namespace (if specified)
func (a *k8sAccess) ListClusterConfigurations(ctx context.Context, namespace string,
	logger logr.Logger) (*configv1beta1.ClusterConfigurationList, error) {

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
	}

	logger.V(logs.LogDebug).Info("Get all ClusterConfigurations")
	clusterConfigurations := &configv1beta1.ClusterConfigurationList{}
	err := a.client.List(ctx, clusterConfigurations, listOptions...)
	return clusterConfigurations, err
}

// GetClusterConfiguration returns current ClusterConfiguration for a given Cluster
func (a *k8sAccess) GetClusterConfiguration(ctx context.Context,
	clusterNamespace, clusterName string, logger logr.Logger) (*configv1beta1.ClusterConfiguration, error) {

	logger = logger.WithValues("namespace", clusterNamespace, "cluster", clusterName)
	logger.V(logs.LogDebug).Info("Get ClusterConfiguration")
	clusterConfiguration := &configv1beta1.ClusterConfiguration{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName},
		clusterConfiguration)
	return clusterConfiguration, err
}

// GetHelmReleases returns list of helm releases deployed in a given cluster
func (a *k8sAccess) GetHelmReleases(clusterConfiguration *configv1beta1.ClusterConfiguration,
	logger logr.Logger) map[configv1beta1.Chart][]string {

	logger = logger.WithValues("namespace", clusterConfiguration.Namespace,
		"clusterConfiguration", clusterConfiguration.Name)

	results := make(map[configv1beta1.Chart][]string)

	logger.V(logs.LogDebug).Info("Get Helm Releases deployed in the cluster")
	for i := range clusterConfiguration.Status.ClusterProfileResources {
		r := clusterConfiguration.Status.ClusterProfileResources[i]
		a.addDeployedCharts(configv1beta1.ClusterProfileKind, r.ClusterProfileName, r.Features, results)
	}
	for i := range clusterConfiguration.Status.ProfileResources {
		r := clusterConfiguration.Status.ProfileResources[i]
		a.addDeployedCharts(configv1beta1.ProfileKind, r.ProfileName, r.Features, results)
	}

	return results
}

// GetResources returns list of resources deployed in a given cluster
func (a *k8sAccess) GetResources(clusterConfiguration *configv1beta1.ClusterConfiguration,
	logger logr.Logger) map[configv1beta1.Resource][]string {

	logger = logger.WithValues("namespace", clusterConfiguration.Namespace,
		"clusterConfiguration", clusterConfiguration.Name)

	results := make(map[configv1beta1.Resource][]string)

	logger.V(logs.LogDebug).Info("Get resources deployed in the cluster")
	for i := range clusterConfiguration.Status.ClusterProfileResources {
		r := clusterConfiguration.Status.ClusterProfileResources[i]
		a.addDeployedResources(configv1beta1.ClusterProfileKind, r.ClusterProfileName, r.Features, results)
	}
	for i := range clusterConfiguration.Status.ProfileResources {
		r := clusterConfiguration.Status.ProfileResources[i]
		a.addDeployedResources(configv1beta1.ProfileKind, r.ProfileName, r.Features, results)
	}

	return results
}

func (a *k8sAccess) addDeployedCharts(profileKind, profileName string,
	features []configv1beta1.Feature, results map[configv1beta1.Chart][]string) {

	for i := range features {
		a.addDeployedChartsForFeature(
			fmt.Sprintf("%s/%s", profileKind, profileName),
			features[i].Charts, results)
	}
}

func (a *k8sAccess) addDeployedChartsForFeature(clusterProfilesName string,
	charts []configv1beta1.Chart, results map[configv1beta1.Chart][]string) {

	for i := range charts {
		chart := &charts[i]
		if v, ok := results[*chart]; ok {
			v = append(v, clusterProfilesName)
			results[*chart] = v
		} else {
			results[*chart] = []string{clusterProfilesName}
		}
	}
}

func (a *k8sAccess) addDeployedResources(profilesKind, profileName string,
	features []configv1beta1.Feature, results map[configv1beta1.Resource][]string) {

	for i := range features {
		a.addDeployedResourcesForFeature(
			fmt.Sprintf("%s/%s", profilesKind, profileName),
			features[i].Resources, results)
	}
}

func (a *k8sAccess) addDeployedResourcesForFeature(profileName string,
	resources []configv1beta1.Resource, results map[configv1beta1.Resource][]string) {

	for i := range resources {
		resource := &resources[i]
		if v, ok := results[*resource]; ok {
			v = append(v, profileName)
			results[*resource] = v
		} else {
			results[*resource] = []string{profileName}
		}
	}
}
