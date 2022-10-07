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

package snapshot

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"

	configv1alpha1 "github.com/projectsveltos/cluster-api-feature-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/logs"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// cluster represents the cluster => namespace/name
	// resourceNamespace and resourceName is the kubernetes resource/helm release namespace/name
	// action represents the type of action (added/deleted/modified or upgraded)
	// message explains better the change
	genSnapshotDiffRow = func(clusterInfo, resourceType, resourceNamespace, resourceName,
		action, message string) []string {
		return []string{
			clusterInfo,
			resourceType,
			resourceNamespace,
			resourceName,
			action,
			message,
		}
	}
)

func doConsiderNamespace(currentNamespace, passedNamespace string) bool {
	if passedNamespace == "" {
		return true
	}

	return currentNamespace == passedNamespace
}

func doConsiderCluster(currentCluster, passedCluster string) bool {
	if passedCluster == "" {
		return true
	}

	return currentCluster == passedCluster
}

// listSnapshotDiffs lists all differences in sampleTwo from sampleOne.
// differences include:
// - list of helm chart (configured, upgraded, removed)
// - list of kubernetes resources (configured, upgraded, removed)
func listSnapshotDiffs(ctx context.Context, snapshotName, fromSample, toSample,
	passedNamespace, passedCluster string,
	logger logr.Logger) error {

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("finding diff between %s and %s", fromSample, toSample))

	// Get the directory containing the collected snapshots for Snapshot instance snapshotName
	instance := utils.GetAccessInstance()
	snapshotInstance := &utilsv1alpha1.Snapshot{}
	err := instance.GetResource(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		return err
	}

	snapshotClient := snapshotter.GetClient()
	artifactFolder, err := snapshotClient.GetCollectedSnapshotFolder(snapshotInstance, logger)
	if err != nil {
		return err
	}

	folderFrom := filepath.Join(*artifactFolder, fromSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(folderFrom)
	if os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			fromSample, snapshotName))
		return err
	}

	folderTo := filepath.Join(*artifactFolder, toSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(folderTo)
	if os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			toSample, snapshotName))
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "ACTION", "MESSAGE"})

	err = listSnapshotDiffsBewteenSamples(folderFrom, folderTo, passedNamespace, passedCluster,
		table, logger)
	if err != nil {
		return err
	}

	table.Render()

	return nil
}

func listSnapshotDiffsBewteenSamples(folderFrom, folderTo, passedNamespace, passedCluster string,
	table *tablewriter.Table, logger logr.Logger) error {

	// Following maps contain per Cluster corresponding ClusterConfiguration at the time snapshot was taken
	// There is one ClusterConfigurations per Cluster
	snapshotClient := snapshotter.GetClient()
	fromClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(folderFrom,
		configv1alpha1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", folderFrom))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(fromClusterConfigurationMap), folderFrom))

	toClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(folderTo,
		configv1alpha1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", folderTo))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(toClusterConfigurationMap), folderFrom))

	err = listFeatureDiff(fromClusterConfigurationMap, toClusterConfigurationMap, passedNamespace, passedCluster,
		table, logger)
	if err != nil {
		return nil
	}

	return nil
}

func listFeatureDiff(fromClusterConfigurationMap, toClusterConfigurationMap map[string][]*unstructured.Unstructured,
	passedNamespace, passedCluster string, table *tablewriter.Table, logger logr.Logger) error {

	for k := range toClusterConfigurationMap {
		if doConsiderNamespace(k, passedNamespace) {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("finding feature diff for clusters in namespace %s", k))
			err := listFeatureDiffInNamespace(k, fromClusterConfigurationMap, toClusterConfigurationMap, passedCluster,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	for k := range fromClusterConfigurationMap {
		if doConsiderNamespace(k, passedNamespace) {
			// If this is present in toClusterConfigurationMap it was already considered
			if _, ok := toClusterConfigurationMap[k]; ok {
				continue
			}
			err := listFeatureDiffInNamespace(k, fromClusterConfigurationMap, toClusterConfigurationMap, passedCluster,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func listFeatureDiffInNamespace(namespace string, fromClusterConfigurationMap, toClusterConfigurationMap map[string][]*unstructured.Unstructured,
	passedCluster string, table *tablewriter.Table, logger logr.Logger) error {

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("get ClusterConfigurations in namespace %s in to folder", namespace))
	toClusterConfigurations, err := getClusterConfigurationsInNamespace(namespace, passedCluster, toClusterConfigurationMap, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("got %d ClusterConfigurations in namespace %s in to folder", len(toClusterConfigurations), namespace))

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("get ClusterConfigurations in namespace %s in from folder", namespace))
	fromClusterConfigurations, err := getClusterConfigurationsInNamespace(namespace, passedCluster, fromClusterConfigurationMap, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("got %d ClusterConfigurations in namespace %s in from folder", len(fromClusterConfigurations), namespace))

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("finding diff for ClusterConfigurations in namespace %s", namespace))
	err = listDiffInClusterConfigurations(fromClusterConfigurations, toClusterConfigurations, table, logger)
	if err != nil {
		return err
	}
	return nil
}

func getClusterConfigurationsInNamespace(namespace, passedCluster string, clusterConfigurationMap map[string][]*unstructured.Unstructured,
	logger logr.Logger) ([]*configv1alpha1.ClusterConfiguration, error) {

	resources, ok := clusterConfigurationMap[namespace]
	if !ok {
		return nil, nil
	}

	results := make([]*configv1alpha1.ClusterConfiguration, 0)
	for i := range resources {
		resource := resources[i]
		var clusterConfiguration configv1alpha1.ClusterConfiguration
		err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(resource.UnstructuredContent(), &clusterConfiguration)
		if err != nil {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to convert unstructured to ClusterConfiguration. Err: %v", err))
			return nil, err
		}
		if doConsiderCluster(clusterConfiguration.Name, passedCluster) {
			results = append(results, &clusterConfiguration)
		}
	}

	return results, nil
}

func listDiffInClusterConfigurations(fromClusterConfigurations, toClusterConfigurations []*configv1alpha1.ClusterConfiguration,
	table *tablewriter.Table, logger logr.Logger) error {

	// Create maps
	fromClusterConfigurationMaps := make(map[string]*configv1alpha1.ClusterConfiguration)
	for i := range fromClusterConfigurations {
		cc := fromClusterConfigurations[i]
		fromClusterConfigurationMaps[cc.Name] = cc
	}

	toClusterConfigurationMaps := make(map[string]*configv1alpha1.ClusterConfiguration)
	for i := range toClusterConfigurations {
		cc := toClusterConfigurations[i]
		toClusterConfigurationMaps[cc.Name] = cc
	}

	for to := range toClusterConfigurationMaps {
		listClusterConfigurationDiff(fromClusterConfigurationMaps[to], toClusterConfigurationMaps[to], table, logger)
	}

	for from := range fromClusterConfigurationMaps {
		if _, ok := toClusterConfigurationMaps[from]; !ok {
			listClusterConfigurationDiff(fromClusterConfigurationMaps[from], toClusterConfigurationMaps[from], table, logger)
		}
	}

	return nil
}

func listClusterConfigurationDiff(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration,
	table *tablewriter.Table, logger logr.Logger) {

	toCharts := make([]configv1alpha1.Chart, 0)
	toResources := make([]configv1alpha1.Resource, 0)
	if toClusterConfiguration != nil {
		for i := range toClusterConfiguration.Status.ClusterProfileResources {
			cpr := &toClusterConfiguration.Status.ClusterProfileResources[i]
			toCharts, toResources = appendChartsAndResources(cpr, toCharts, toResources)
		}
	}

	fromCharts := make([]configv1alpha1.Chart, 0)
	fromResources := make([]configv1alpha1.Resource, 0)
	if fromClusterConfiguration != nil {
		for i := range fromClusterConfiguration.Status.ClusterProfileResources {
			cpr := &fromClusterConfiguration.Status.ClusterProfileResources[i]
			fromCharts, fromResources = appendChartsAndResources(cpr, fromCharts, fromResources)
		}
	}

	// Evaluate the diff
	chartAdded, chartModified, chartDeleted, modifiedChartMessage :=
		chartDifference(fromCharts, toCharts)
	resourceAdded, resourceModified, resourceDeleted, modifiedResourceMessage :=
		resourceDifference(fromResources, toResources)

	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartAdded, "added", nil, table)
	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartModified, "modified", modifiedChartMessage, table)
	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartDeleted, "deleted", nil, table)

	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceAdded, "added", nil, table)
	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceModified, "modified", modifiedResourceMessage, table)
	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceDeleted, "deleted", nil, table)
}

func addChartEntry(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration,
	charts []*configv1alpha1.Chart, action string, message map[configv1alpha1.Chart]string, table *tablewriter.Table) {

	clusterInfo := func(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration) string {
		if toClusterConfiguration != nil {
			return fmt.Sprintf("%s/%s", toClusterConfiguration.Namespace, toClusterConfiguration.Name)
		}
		return fmt.Sprintf("%s/%s", fromClusterConfiguration.Namespace, fromClusterConfiguration.Name)
	}

	for i := range charts {
		msg := ""
		if message != nil {
			msg = message[*charts[i]]
		}

		table.Append(genSnapshotDiffRow(
			clusterInfo(fromClusterConfiguration, toClusterConfiguration),
			"helm release",
			charts[i].Namespace, charts[i].ReleaseName,
			action, msg))
	}
}

func addResourceEntry(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration,
	resources []*configv1alpha1.Resource, action string, message map[configv1alpha1.Resource]string,
	table *tablewriter.Table) {

	clusterInfo := func(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration) string {
		if toClusterConfiguration != nil {
			return fmt.Sprintf("%s/%s", toClusterConfiguration.Namespace, toClusterConfiguration.Name)
		}
		return fmt.Sprintf("%s/%s", fromClusterConfiguration.Namespace, fromClusterConfiguration.Name)
	}

	for i := range resources {
		msg := ""
		if message != nil {
			msg = message[*resources[i]]
		}
		table.Append(genSnapshotDiffRow(
			clusterInfo(fromClusterConfiguration, toClusterConfiguration),
			fmt.Sprintf("%s/%s", resources[i].Group, resources[i].Kind),
			resources[i].Namespace, resources[i].Name,
			action, msg))
	}
}

// chartDifference returns differences between from and to
func chartDifference(from, to []configv1alpha1.Chart) (added, modified, deleted []*configv1alpha1.Chart,
	modifiedMessage map[configv1alpha1.Chart]string) {

	modifiedMessage = make(map[configv1alpha1.Chart]string)

	chartInfo := func(chart *configv1alpha1.Chart) string {
		return fmt.Sprintf("%s/%s", chart.Namespace, chart.ReleaseName)
	}

	toChartMap := make(map[string]*configv1alpha1.Chart, len(to))
	for i := range to {
		chart := &to[i]
		toChartMap[chartInfo(chart)] = chart
	}

	fromChartMap := make(map[string]*configv1alpha1.Chart, len(from))
	for i := range from {
		chart := &from[i]
		fromChartMap[chartInfo(chart)] = chart
	}

	for k := range toChartMap {
		v, ok := fromChartMap[k]
		if !ok {
			added = append(added, toChartMap[k])
		} else if !reflect.DeepEqual(v, toChartMap[k]) {
			modified = append(modified, toChartMap[k])
			msg := fmt.Sprintf("To version: %s ", toChartMap[k].ChartVersion)
			msg += fmt.Sprintf("From version %s", fromChartMap[k].ChartVersion)
			modifiedMessage[*toChartMap[k]] = msg
		}
	}

	for k := range fromChartMap {
		_, ok := toChartMap[k]
		if !ok {
			deleted = append(deleted, fromChartMap[k])
		}
	}

	return added, modified, deleted, modifiedMessage
}

// resourceDifference returns differences between from and to
func resourceDifference(from, to []configv1alpha1.Resource) (added, modified, deleted []*configv1alpha1.Resource,
	modifiedMessage map[configv1alpha1.Resource]string) {

	resourceInfo := func(resource *configv1alpha1.Resource) string {
		return fmt.Sprintf("%s:%s:%s/%s", resource.Group, resource.Kind, resource.Namespace, resource.Name)
	}

	toResourceMap := make(map[string]*configv1alpha1.Resource, len(to))
	for i := range to {
		resource := &to[i]
		toResourceMap[resourceInfo(resource)] = resource
	}

	fromResourceMap := make(map[string]*configv1alpha1.Resource, len(from))
	for i := range from {
		resource := &from[i]
		fromResourceMap[resourceInfo(resource)] = resource
	}

	var addedResources []*configv1alpha1.Resource
	var deletedResources []*configv1alpha1.Resource
	var modifiedResources []*configv1alpha1.Resource
	modifiedMessage = make(map[configv1alpha1.Resource]string)

	for k := range toResourceMap {
		v, ok := fromResourceMap[k]
		if !ok {
			addedResources = append(addedResources, toResourceMap[k])
		} else if !reflect.DeepEqual(v, toResourceMap[k]) {
			modifiedResources = append(modifiedResources, toResourceMap[k])
			msg := fmt.Sprintf("To see diff compare %s %s/%s in the from folder ", v.Owner.Kind, v.Owner.Namespace, v.Owner.Name)
			msg += fmt.Sprintf("with %s %s/%s in the to folder", toResourceMap[k].Owner.Kind, toResourceMap[k].Owner.Namespace, toResourceMap[k].Owner.Name)
			modifiedMessage[*toResourceMap[k]] = msg
		}
	}

	for k := range fromResourceMap {
		_, ok := toResourceMap[k]
		if !ok {
			deletedResources = append(deletedResources, fromResourceMap[k])
		}
	}

	return addedResources, modifiedResources, deletedResources, modifiedMessage
}

func appendChartsAndResources(cpr *configv1alpha1.ClusterProfileResource, charts []configv1alpha1.Chart,
	resources []configv1alpha1.Resource) ([]configv1alpha1.Chart, []configv1alpha1.Resource) {

	for i := range cpr.Features {
		for j := range cpr.Features[i].Charts {
			c := cpr.Features[i].Charts[j]
			c.LastAppliedTime = nil
			charts = append(charts, c)
		}
		for j := range cpr.Features[i].Resources {
			r := cpr.Features[i].Resources[j]
			resources = append(resources, r)
		}
	}

	return charts, resources
}

// Diff lists differences between two snapshots
func Diff(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
	sveltosctl snapshot diff [options] --snapshot=<name> --from-sample=<name> --to-sample=<name> [--namespace=<name>] [--cluster=<name>] [--verbose]

     --snapshot=<name>      Name of the Snapshot instance
     --from-sample=<name>   Name of the directory containing this sample (use sveltosctl snapshot list to see all collected snapshosts)
     --to-sample=<name>     Name of the directory containing this sample (use sveltosctl snapshot list to see all collected snapshosts)
     --namespace=<name>     Show features differences for clusters in this namespace. If not specified all namespaces are considered.
     --cluster=<name>       Show features differences for clusters with name. If not specified all cluster names are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The snapshot diff command list differences in deployed features in sample-two having sample-one as starting point
`

	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogVerbose).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogVerbose))
		if err != nil {
			return err
		}
	}

	logger = klogr.New()

	snapshostName := parsedArgs["--snapshot"].(string)
	toSample := parsedArgs["--to-sample"].(string)
	fromSample := parsedArgs["--from-sample"].(string)

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	return listSnapshotDiffs(ctx, snapshostName, fromSample, toSample, namespace, cluster, logger)
}
