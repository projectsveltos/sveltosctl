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
	// resourceNamespace and resourceName is the helm release namespace/name
	// currentVersion is the helm chart version currently deployed
	// newerVersion and newerPatchVersion are the highest version and highest same-minor
	// patch version published upstream, if newer than currentVersion
	genHelmUpdateRow = func(cluster, resourceNamespace, resourceName,
		currentVersion, newerVersion, newerPatchVersion string) []string {
		return []string{
			cluster,
			resourceNamespace,
			resourceName,
			currentVersion,
			newerVersion,
			newerPatchVersion,
		}
	}
)

func displayHelmUpdates(ctx context.Context, passedNamespace, passedCluster, passedClusterType string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "NAMESPACE", "RELEASE NAME", "CURRENT VERSION", "NEWER VERSION", "NEWER PATCH VERSION")

	if err := displayHelmUpdatesInNamespaces(ctx, passedNamespace, passedCluster,
		passedClusterType, table, logger); err != nil {
		return err
	}

	return table.Render()
}

func displayHelmUpdatesInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedClusterType string,
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
			err = displayHelmUpdatesInNamespace(ctx, ns.Name, passedCluster, passedClusterType, table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayHelmUpdatesInNamespace(ctx context.Context, namespace, passedCluster, passedClusterType string,
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
		if doConsiderClusterConfiguration(cc, passedCluster) &&
			doConsiderClusterType(clusterConfigurationType(cc), passedClusterType) {

			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering ClusterConfiguration: %s", cc.Name))
			if err := displayHelmUpdatesForCluster(ctx, cc, table, logger); err != nil {
				return err
			}
		}
	}

	return nil
}

// clusterConfigurationType returns the cluster type (Capi/Sveltos) a ClusterConfiguration
// was generated for.
func clusterConfigurationType(clusterConfiguration *configv1beta1.ClusterConfiguration) string {
	if clusterConfiguration.Labels == nil {
		return ""
	}
	return clusterConfiguration.Labels[configv1beta1.ClusterTypeLabel]
}

func displayHelmUpdatesForCluster(ctx context.Context, clusterConfiguration *configv1beta1.ClusterConfiguration,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger = logger.WithValues("clusterConfiguration", clusterConfiguration.Name)
	logger.V(logs.LogDebug).Info("Get ClusterConfiguration")

	clusterName := instance.GetClusterNameFromClusterConfiguration(clusterConfiguration)
	clusterType := clusterConfigurationType(clusterConfiguration)
	clusterInfo := fmt.Sprintf("%s/%s", clusterConfiguration.Namespace, clusterName)

	outdatedInfo, err := instance.GetOutdatedHelmChartInfo(ctx, clusterConfiguration.Namespace,
		clusterName, clusterType, logger)
	if err != nil {
		return err
	}

	if len(outdatedInfo) == 0 {
		return nil
	}

	helmCharts := instance.GetHelmReleases(clusterConfiguration, logger)
	for chart := range helmCharts {
		info, ok := outdatedInfo[utils.HelmReleaseKey(chart.Namespace, chart.ReleaseName)]
		if !ok {
			continue
		}

		if err := table.Append(genHelmUpdateRow(clusterInfo, chart.Namespace, chart.ReleaseName,
			chart.ChartVersion, info.LatestVersion, info.LatestPatchVersion)); err != nil {
			return err
		}
	}

	return nil
}

// HelmUpdates displays information about Helm charts deployed in clusters for which a
// newer version or same-minor patch is available upstream
func HelmUpdates(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show helm-updates [options] [--namespace=<name>] [--cluster=<name>] [--cluster-type=<type>] [--verbose]

     --namespace=<name>      Show outdated helm charts deployed in clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show outdated helm charts deployed in cluster with name.
                             If not specified all cluster names are considered.
     --cluster-type=<type>   Show outdated helm charts deployed in cluster with this type
                             (Capi or Sveltos). If not specified all cluster types are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The show helm-updates command shows, for each Helm chart currently deployed in a
  cluster, the newer version and/or newer same-minor patch version available upstream.
  Charts already on the latest version are omitted.
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

	clusterType := ""
	if passedClusterType := parsedArgs["--cluster-type"]; passedClusterType != nil {
		clusterType = passedClusterType.(string)
	}

	return displayHelmUpdates(ctx, namespace, cluster, clusterType, logger)
}
