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

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// cluster represents the cluster => namespace/name
	// resourceNamespace and resourceName is the kubernetes resource/helm release namespace/name
	// resourceVersion applies to helm releases only and it is the helm chart version
	// lastApplied represents the time resource was updated
	// clusterProfileNames is the list of all ClusterProfiles causing the resource to be deployed
	// in the cluster
	genAddOnsRow = func(cluster, resourceType, resourceNamespace, resourceName, resourceVersion,
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

func displayAddOns(ctx context.Context, passedNamespace, passedCluster, passedProfile string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "VERSION", "TIME", "PROFILES"})

	if err := displayAddOnsInNamespaces(ctx, passedNamespace, passedCluster,
		passedProfile, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displayAddOnsInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedProfile string,
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
			err = displayAddOnsInNamespace(ctx, ns.Name, passedCluster, passedProfile,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayAddOnsInNamespace(ctx context.Context, namespace, passedCluster, passedProfile string,
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
			displayAddOnsForCluster(cc, passedProfile, table, logger)
		}
	}

	return nil
}

func displayAddOnsForCluster(clusterConfiguration *configv1beta1.ClusterConfiguration, passedProfile string,
	table *tablewriter.Table, logger logr.Logger) {

	instance := utils.GetAccessInstance()
	helmCharts := instance.GetHelmReleases(clusterConfiguration, logger)

	logger = logger.WithValues("clusterConfiguration", clusterConfiguration.Name)
	logger.V(logs.LogDebug).Info("Get ClusterConfiguration")

	clusterName := instance.GetClusterNameFromClusterConfiguration(clusterConfiguration)

	clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterName)
	for chart := range helmCharts {
		if doConsiderProfile(helmCharts[chart], passedProfile) {
			table.Append(genAddOnsRow(clusterInfo, "helm chart", chart.Namespace, chart.ReleaseName, chart.ChartVersion,
				chart.LastAppliedTime.String(), helmCharts[chart]))
		}
	}

	resources := instance.GetResources(clusterConfiguration, logger)
	for resource := range resources {
		if doConsiderProfile(resources[resource], passedProfile) {
			table.Append(genAddOnsRow(clusterInfo, fmt.Sprintf("%s:%s", resource.Group, resource.Kind),
				resource.Namespace, resource.Name, "N/A",
				resource.LastAppliedTime.String(), resources[resource]))
		}
	}
}

// AddOns displays information about Kubernetes AddOns deployed in clusters
func AddOns(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show addons [options] [--namespace=<name>] [--cluster=<name>] [--profile=<name>] [--verbose]

     --namespace=<name>      Show Kubernetes addons deployed in clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show Kubernetes addons deployed in cluster with name.
                             If not specified all cluster names are considered.
     --profile=<kind/name>   Show Kubernetes addons deployed because of this clusterprofile/profile.
                             If not specified all clusterprofiles/profiles are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show addons command shows information about Kubernetes addons deployed in clusters.
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

	profile := ""
	if passedProfile := parsedArgs["--profile"]; passedProfile != nil {
		profile = passedProfile.(string)
	}

	return displayAddOns(ctx, namespace, cluster, profile, logger)
}
