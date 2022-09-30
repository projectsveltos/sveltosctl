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

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/logs"
)

// ListClusterConfigurations returns all current ClusterConfigurations
func (a *k8sAccess) ListClusterConfigurations(ctx context.Context, namespace string,
	logger logr.Logger) (*configv1alpha1.ClusterConfigurationList, error) {

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
	}

	logger.V(logs.LogVerbose).Info("Get all ClusterConfigurations")
	clusterConfigurations := &configv1alpha1.ClusterConfigurationList{}
	err := a.client.List(ctx, clusterConfigurations, listOptions...)
	return clusterConfigurations, err
}

// GetClusterConfiguration returns current ClusterConfiguration for a given Cluster
func (a *k8sAccess) GetClusterConfiguration(ctx context.Context,
	clusterNamespace, clusterName string, logger logr.Logger) (*configv1alpha1.ClusterConfiguration, error) {

	logger = logger.WithValues("namespace", clusterNamespace, "cluster", clusterName)
	logger.V(logs.LogVerbose).Info("Get ClusterConfiguration")
	clusterConfiguration := &configv1alpha1.ClusterConfiguration{}
	err := a.client.Get(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName},
		clusterConfiguration)
	return clusterConfiguration, err
}

// GetHelmReleases returns list of helm releases deployed in a given cluster
func (a *k8sAccess) GetHelmReleases(clusterConfiguration *configv1alpha1.ClusterConfiguration,
	logger logr.Logger) map[configv1alpha1.Chart][]string {

	logger = logger.WithValues("namespace", clusterConfiguration.Namespace,
		"clusterConfiguration", clusterConfiguration.Name)

	results := make(map[configv1alpha1.Chart][]string)

	logger.V(logs.LogVerbose).Info("Get Helm Releases deployed in the cluster")
	for i := range clusterConfiguration.Status.ClusterFeatureResources {
		r := clusterConfiguration.Status.ClusterFeatureResources[i]
		a.addDeployedCharts(r.ClusterFeatureName, r.Features, results)
	}

	return results
}

// GetResources returns list of resources deployed in a given cluster
func (a *k8sAccess) GetResources(clusterConfiguration *configv1alpha1.ClusterConfiguration,
	logger logr.Logger) map[configv1alpha1.Resource][]string {

	logger = logger.WithValues("namespace", clusterConfiguration.Namespace,
		"clusterConfiguration", clusterConfiguration.Name)

	results := make(map[configv1alpha1.Resource][]string)

	logger.V(logs.LogVerbose).Info("Get resources deployed in the cluster")
	for i := range clusterConfiguration.Status.ClusterFeatureResources {
		r := clusterConfiguration.Status.ClusterFeatureResources[i]
		a.addDeployedResources(r.ClusterFeatureName, r.Features, results)
	}

	return results
}

func (a *k8sAccess) addDeployedCharts(clusterFeaturesName string,
	features []configv1alpha1.Feature, results map[configv1alpha1.Chart][]string) {

	for i := range features {
		a.addDeployedChartsForFeature(clusterFeaturesName, features[i].Charts, results)
	}
}

func (a *k8sAccess) addDeployedChartsForFeature(clusterFeaturesName string,
	charts []configv1alpha1.Chart, results map[configv1alpha1.Chart][]string) {

	for i := range charts {
		chart := &charts[i]
		if v, ok := results[*chart]; ok {
			v = append(v, clusterFeaturesName)
			results[*chart] = v
		} else {
			results[*chart] = []string{clusterFeaturesName}
		}
	}
}

func (a *k8sAccess) addDeployedResources(clusterFeaturesName string,
	features []configv1alpha1.Feature, results map[configv1alpha1.Resource][]string) {

	for i := range features {
		a.addDeployedResourcesForFeature(clusterFeaturesName, features[i].Resources, results)
	}
}

func (a *k8sAccess) addDeployedResourcesForFeature(clusterFeaturesName string,
	resources []configv1alpha1.Resource, results map[configv1alpha1.Resource][]string) {

	for i := range resources {
		resource := &resources[i]
		if v, ok := results[*resource]; ok {
			v = append(v, clusterFeaturesName)
			results[*resource] = v
		} else {
			results[*resource] = []string{clusterFeaturesName}
		}
	}
}
