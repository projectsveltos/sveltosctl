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

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// cluster represents the cluster => namespace/name
	// resourceNamespace and resourceName is the kubernetes resource/helm release namespace/name
	// resourceVersion applies to helm releases only and it is the helm chart version
	// lastApplied represents the time resource was updated
	// clusterProfileNames is the list of all ClusterProfiles causing the resource to be deployed
	// in the cluster
	genFeatureRow = func(cluster, resourceType, resourceNamespace, resourceName, resourceVersion,
		lastApplied string, clusterProfileNames []string) []string {
		clusterProfiles := strings.Join(clusterProfileNames, ";")
		return []string{
			cluster,
			resourceType,
			resourceNamespace,
			resourceName,
			resourceVersion,
			lastApplied,
			clusterProfiles,
		}
	}
)

func displayFeatures(ctx context.Context, passedNamespace, passedCluster, passedClusterProfile string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "VERSION", "TIME", "CLUSTER PROFILES"})

	if err := displayFeaturesInNamespaces(ctx, passedNamespace, passedCluster,
		passedClusterProfile, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displayFeaturesInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedClusterProfile string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	namespaces, err := instance.ListNamespaces(ctx, logger)
	if err != nil {
		return err
	}

	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if doConsiderNamespace(ns, passedNamespace) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering namespace: %s", ns.Name))
			err = displayFeaturesInNamespace(ctx, ns.Name, passedCluster, passedClusterProfile,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayFeaturesInNamespace(ctx context.Context, namespace, passedCluster, passedClusterProfile string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger = logger.WithValues("namespace", namespace)
	logger.V(logs.LogDebug).Info("Get all ClusterConfiguration")
	clusterConfigurations, err := instance.ListClusterConfigurations(ctx, namespace, logger)
	if err != nil {
		return err
	}

	for i := range clusterConfigurations.Items {
		cc := &clusterConfigurations.Items[i]
		if doConsiderClusterConfiguration(cc, passedCluster) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering ClusterConfiguration: %s", cc.Name))
			displayFeaturesForCluster(cc, passedClusterProfile, table, logger)
		}
	}

	return nil
}

func displayFeaturesForCluster(clusterConfiguration *configv1alpha1.ClusterConfiguration, passedClusterProfile string,
	table *tablewriter.Table, logger logr.Logger) {

	instance := utils.GetAccessInstance()
	helmCharts := instance.GetHelmReleases(clusterConfiguration, logger)

	logger = logger.WithValues("clusterConfiguration", clusterConfiguration.Name)
	logger.V(logs.LogDebug).Info("Get ClusterConfiguration")
	clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterConfiguration.Name)
	for chart := range helmCharts {
		if doConsiderClusterProfile(helmCharts[chart], passedClusterProfile) {
			table.Append(genFeatureRow(clusterInfo, "helm chart", chart.Namespace, chart.ReleaseName, chart.ChartVersion,
				chart.LastAppliedTime.String(), helmCharts[chart]))
		}
	}

	resources := instance.GetResources(clusterConfiguration, logger)
	for resource := range resources {
		if doConsiderClusterProfile(resources[resource], passedClusterProfile) {
			table.Append(genFeatureRow(clusterInfo, fmt.Sprintf("%s:%s", resource.Group, resource.Kind),
				resource.Namespace, resource.Name, "N/A",
				resource.LastAppliedTime.String(), resources[resource]))
		}
	}
}

// Features displays information about features deployed in clusters
func Features(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show features [options] [--namespace=<name>] [--cluster=<name>] [--clusterprofile=<name>] [--verbose]

     --namespace=<name>      Show features deployed in clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show features deployed in cluster with name.
                             If not specified all cluster names are considered.
     --clusterprofile=<name> Show features deployed because of this clusterprofile.
                             If not specified all clusterprofile names are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show features command shows information about features deployed in clusters.
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

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug))
		if err != nil {
			return err
		}
	}

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	clusterProfile := ""
	if passedClusterProfile := parsedArgs["--clusterprofile"]; passedClusterProfile != nil {
		clusterProfile = passedClusterProfile.(string)
	}

	return displayFeatures(ctx, namespace, cluster, clusterProfile, logger)
}
