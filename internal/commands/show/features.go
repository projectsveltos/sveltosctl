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

package show

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/logs"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// cluster represents the cluster => namespace/name
	// resourceNamespace and resourceName is the kubernetes resource/helm release namespace/name
	// resourceVersion applies to helm releases only and it is the helm chart version
	// lastApplies represents the time resource was updated
	// clusterFeatureNames is the list of all ClusterFeatures causing the resource to be deployed
	// in the cluster
	genFeatureRow = func(cluster, resourceType, resourceNamespace, resourceName, resourceVersion,
		lastApplied string, clusterFeatureNames []string) []string {
		clusterFeatures := strings.Join(clusterFeatureNames, ";")
		return []string{
			cluster,
			resourceType,
			resourceNamespace,
			resourceName,
			resourceVersion,
			lastApplied,
			clusterFeatures,
		}
	}
)

func doConsiderNamespace(ns *corev1.Namespace, passedNamespace string) bool {
	if passedNamespace == "" {
		return true
	}

	return ns.Name == passedNamespace
}

func doConsiderClusterConfiguration(clusterConfiguration *configv1alpha1.ClusterConfiguration,
	passedCluster string) bool {

	if passedCluster == "" {
		return true
	}

	return clusterConfiguration.Name == passedCluster
}

func doConsiderFeature(clusterFeatureNames []string, passedClusterFeature string) bool {
	if passedClusterFeature == "" {
		return true
	}

	for i := range clusterFeatureNames {
		if clusterFeatureNames[i] == passedClusterFeature {
			return true
		}
	}

	return false
}

func displayFeatures(ctx context.Context, passedNamespace, passedCluster, passedClusterFeature string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "VERSION", "TIME", "CLUSTER FEATURES"})

	if err := displayFeaturesInNamespaces(ctx, passedNamespace, passedCluster,
		passedClusterFeature, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displayFeaturesInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedClusterFeature string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	namespaces, err := instance.ListNamespaces(ctx, logger)
	if err != nil {
		return err
	}

	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if doConsiderNamespace(ns, passedNamespace) {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Considering namespace: %s", ns.Name))
			err = displayFeaturesInNamespace(ctx, ns.Name, passedCluster, passedClusterFeature,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayFeaturesInNamespace(ctx context.Context, namespace, passedCluster, passedClusterFeature string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger = logger.WithValues("namespace", namespace)
	logger.V(logs.LogVerbose).Info("Get all ClusterConfiguration")
	clusterConfigurations, err := instance.ListClusterConfigurations(ctx, namespace, logger)
	if err != nil {
		return err
	}

	for i := range clusterConfigurations.Items {
		cc := &clusterConfigurations.Items[i]
		if doConsiderClusterConfiguration(cc, passedCluster) {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Considering ClusterConfiguration: %s", cc.Name))
			displayFeaturesForCluster(cc, passedClusterFeature, table, logger)
		}
	}

	return nil
}

func displayFeaturesForCluster(clusterConfiguration *configv1alpha1.ClusterConfiguration, passedClusterFeature string,
	table *tablewriter.Table, logger logr.Logger) {

	instance := utils.GetAccessInstance()
	helmCharts := instance.GetHelmReleases(clusterConfiguration, logger)

	logger = logger.WithValues("clusterConfiguration", clusterConfiguration.Name)
	logger.V(logs.LogVerbose).Info("Get ClusterConfiguration")
	clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterConfiguration.Name)
	for chart := range helmCharts {
		if doConsiderFeature(helmCharts[chart], passedClusterFeature) {
			table.Append(genFeatureRow(clusterInfo, "helm chart", chart.Namespace, chart.ReleaseName, chart.ChartVersion,
				chart.LastAppliedTime.String(), helmCharts[chart]))
		}
	}

	resources := instance.GetResources(clusterConfiguration, logger)
	for resource := range resources {
		if doConsiderFeature(resources[resource], passedClusterFeature) {
			table.Append(genFeatureRow(clusterInfo, fmt.Sprintf("%s:%s", resource.Group, resource.Kind),
				resource.Namespace, resource.Name, "N/A",
				resource.LastAppliedTime.String(), resources[resource]))
		}
	}
}

// Features displays information about features deployed in clusters
func Features(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show features [--namespace=<name>] [--cluster=<name>] [--clusterfeature=<name>] [--verbose]
Options:
  -h --help                  Show this screen.
     --namespace=<name>      Show features deployed in clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>        Show features deployed in cluster with name. If not specified all cluster names are considered.
	 --clusterfeature=<name> Show features deployed because of this clusterfeature. If not specified all clusterfeature names are considered.
     --verbose               Verbose mode. Print each step.

Description:
  The show cluster command shows information about workload cluster.
`
	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}
	if len(parsedArgs) == 0 {
		return nil
	}
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogVerbose))
		if err != nil {
			return err
		}
	}
	defer func() {
		_ = flag.Lookup("v").Value.Set(fmt.Sprint(0))
	}()

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	clusterFeature := ""
	if passedClusterFeature := parsedArgs["--clusterfeature"]; passedClusterFeature != nil {
		clusterFeature = passedClusterFeature.(string)
	}

	return displayFeatures(ctx, namespace, cluster, clusterFeature, logger)
}
