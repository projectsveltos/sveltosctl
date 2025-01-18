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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
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

	genDiffRow = func(objectName, action string) []string {
		return []string{
			objectName,
			action,
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
	passedNamespace, passedCluster string, rawDiff bool,
	logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("finding diff between %s and %s", fromSample, toSample))

	// Get the directory containing the collected snapshots for Snapshot instance snapshotName
	instance := utils.GetAccessInstance()
	snapshotInstance := &utilsv1beta1.Snapshot{}
	err := instance.GetResource(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		return err
	}

	snapshotClient := collector.GetClient()
	artifactFolder, err := snapshotClient.GetFolder(snapshotInstance.Spec.Storage, snapshotInstance.Name,
		collector.Snapshot, logger)
	if err != nil {
		return err
	}

	fromFolder := filepath.Join(*artifactFolder, fromSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(fromFolder)
	if os.IsNotExist(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			fromSample, snapshotName))
		return err
	}

	toFolder := filepath.Join(*artifactFolder, toSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(toFolder)
	if os.IsNotExist(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			toSample, snapshotName))
		return err
	}

	err = listClusterResourcesDiff(fromFolder, toFolder, passedNamespace, passedCluster, rawDiff, logger)
	if err != nil {
		return err
	}

	err = listDiff(fromFolder, toFolder, libsveltosv1beta1.ClassifierKind, rawDiff, logger)
	if err != nil {
		return err
	}

	err = listDiff(fromFolder, toFolder, libsveltosv1beta1.RoleRequestKind, rawDiff, logger)
	if err != nil {
		return err
	}

	return nil
}

func listDiff(fromFolder, toFolder, kind string, rawDiff bool, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	froms, err := snapshotClient.GetClusterResources(fromFolder, kind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect %ss from folder %s", kind, fromFolder))
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d %ss in folder %s", len(froms), kind, fromFolder))

	tos, err := snapshotClient.GetClusterResources(toFolder, kind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect %ss from folder %s", kind, toFolder))
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d %s in folder %s", len(tos), kind, toFolder))

	fromMap := make(map[string]*unstructured.Unstructured, len(froms))
	for i := range froms {
		cl := froms[i]
		fromMap[cl.GetName()] = cl
	}

	toMap := make(map[string]*unstructured.Unstructured, len(tos))
	for i := range tos {
		cl := tos[i]
		toMap[cl.GetName()] = cl
	}

	err = showDiff(fromMap, toMap, kind, rawDiff, logger)
	if err != nil {
		return err
	}

	return nil
}

func showDiff(fromMap, toMap map[string]*unstructured.Unstructured, kind string, rawDiff bool,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{strings.ToUpper(kind), "ACTION"})

	for clName := range toMap {
		v, ok := fromMap[clName]
		if !ok {
			table.Append(genDiffRow(clName, "added"))
			continue
		}
		hasDiff, err := hasResourceDiff(v, toMap[clName], rawDiff, logger)
		if err != nil {
			return err
		}
		if hasDiff {
			table.Append(genDiffRow(clName, "modified"))
		}
	}

	for clName := range fromMap {
		_, ok := toMap[clName]
		if !ok {
			table.Append(genDiffRow(clName, "removed"))
		}
	}

	if !rawDiff {
		if table.NumLines() > 0 {
			table.Render()
		}
	}

	return nil
}

func listClusterResourcesDiff(fromFolder, toFolder, passedNamespace, passedCluster string, rawDiff bool,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "ACTION", "MESSAGE"})

	err := listSnapshotDiffsBewteenSamples(fromFolder, toFolder, passedNamespace, passedCluster, rawDiff,
		table, logger)
	if err != nil {
		return err
	}

	if !rawDiff {
		if table.NumLines() > 0 {
			table.Render()
		} else {
			//nolint: forbidigo // indicating no diff
			fmt.Println("no changes detected in clusters")
		}
	}

	return nil
}

func listSnapshotDiffsBewteenSamples(fromFolder, toFolder, passedNamespace, passedCluster string,
	rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	// Following maps contain per Cluster corresponding ClusterConfiguration at the time snapshot was taken
	// There is one ClusterConfigurations per Cluster
	snapshotClient := collector.GetClient()
	fromClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(fromFolder,
		configv1beta1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", fromFolder))
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(fromClusterConfigurationMap), fromFolder))

	toClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(toFolder,
		configv1beta1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", toFolder))
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(toClusterConfigurationMap), fromFolder))

	err = listFeatureDiff(fromFolder, toFolder, fromClusterConfigurationMap, toClusterConfigurationMap,
		passedNamespace, passedCluster, rawDiff, table, logger)
	if err != nil {
		return err
	}

	return nil
}

func listFeatureDiff(fromFolder, toFolder string,
	fromClusterConfigurationMap, toClusterConfigurationMap map[string][]*unstructured.Unstructured,
	passedNamespace, passedCluster string, rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	for k := range toClusterConfigurationMap {
		if doConsiderNamespace(k, passedNamespace) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("finding feature diff for clusters in namespace %s", k))
			err := listFeatureDiffInNamespace(fromFolder, toFolder, k, fromClusterConfigurationMap, toClusterConfigurationMap,
				passedCluster, rawDiff, table, logger)
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
			err := listFeatureDiffInNamespace(fromFolder, toFolder, k, fromClusterConfigurationMap, toClusterConfigurationMap,
				passedCluster, rawDiff, table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func listFeatureDiffInNamespace(fromFolder, toFolder, namespace string,
	fromClusterConfigurationMap, toClusterConfigurationMap map[string][]*unstructured.Unstructured,
	passedCluster string, rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("get ClusterConfigurations in namespace %s in to folder", namespace))
	toClusterConfigurations, err := getClusterConfigurationsInNamespace(namespace, passedCluster, toClusterConfigurationMap, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("got %d ClusterConfigurations in namespace %s in to folder", len(toClusterConfigurations), namespace))

	logger.V(logs.LogDebug).Info(fmt.Sprintf("get ClusterConfigurations in namespace %s in from folder", namespace))
	fromClusterConfigurations, err := getClusterConfigurationsInNamespace(namespace, passedCluster, fromClusterConfigurationMap, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("got %d ClusterConfigurations in namespace %s in from folder", len(fromClusterConfigurations), namespace))

	logger.V(logs.LogDebug).Info(fmt.Sprintf("finding diff for ClusterConfigurations in namespace %s", namespace))
	err = listDiffInClusterConfigurations(fromFolder, toFolder, fromClusterConfigurations, toClusterConfigurations, rawDiff, table, logger)
	if err != nil {
		return err
	}
	return nil
}

func getClusterConfigurationsInNamespace(namespace, passedCluster string, clusterConfigurationMap map[string][]*unstructured.Unstructured,
	logger logr.Logger) ([]*configv1beta1.ClusterConfiguration, error) {

	resources, ok := clusterConfigurationMap[namespace]
	if !ok {
		return nil, nil
	}

	results := make([]*configv1beta1.ClusterConfiguration, 0)
	for i := range resources {
		resource := resources[i]
		var clusterConfiguration configv1beta1.ClusterConfiguration
		err := runtime.DefaultUnstructuredConverter.
			FromUnstructured(resource.UnstructuredContent(), &clusterConfiguration)
		if err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to convert unstructured to ClusterConfiguration. Err: %v", err))
			return nil, err
		}
		if doConsiderCluster(clusterConfiguration.Name, passedCluster) {
			results = append(results, &clusterConfiguration)
		}
	}

	return results, nil
}

func listDiffInClusterConfigurations(fromFolder, toFolder string, fromClusterConfigurations, toClusterConfigurations []*configv1beta1.ClusterConfiguration,
	rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	// Create maps
	fromClusterConfigurationMaps := make(map[string]*configv1beta1.ClusterConfiguration)
	for i := range fromClusterConfigurations {
		cc := fromClusterConfigurations[i]
		fromClusterConfigurationMaps[cc.Name] = cc
	}

	toClusterConfigurationMaps := make(map[string]*configv1beta1.ClusterConfiguration)
	for i := range toClusterConfigurations {
		cc := toClusterConfigurations[i]
		toClusterConfigurationMaps[cc.Name] = cc
	}

	for to := range toClusterConfigurationMaps {
		if err := listClusterConfigurationDiff(fromFolder, toFolder, fromClusterConfigurationMaps[to],
			toClusterConfigurationMaps[to], rawDiff, table, logger); err != nil {
			return err
		}
	}

	for from := range fromClusterConfigurationMaps {
		if _, ok := toClusterConfigurationMaps[from]; !ok {
			if err := listClusterConfigurationDiff(fromFolder, toFolder, fromClusterConfigurationMaps[from],
				toClusterConfigurationMaps[from], rawDiff, table, logger); err != nil {
				return err
			}
		}
	}

	return nil
}

func listClusterConfigurationDiff(fromFolder, toFolder string, fromClusterConfiguration, toClusterConfiguration *configv1beta1.ClusterConfiguration,
	rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	toCharts := make([]configv1beta1.Chart, 0)
	toResources := make([]configv1beta1.Resource, 0)
	if toClusterConfiguration != nil {
		for i := range toClusterConfiguration.Status.ClusterProfileResources {
			cpr := &toClusterConfiguration.Status.ClusterProfileResources[i]
			toCharts, toResources = appendChartsAndResourcesForClusterProfiles(cpr, toCharts, toResources)
		}
		for i := range toClusterConfiguration.Status.ProfileResources {
			pr := &toClusterConfiguration.Status.ProfileResources[i]
			toCharts, toResources = appendChartsAndResourcesForProfiles(pr, toCharts, toResources)
		}
	}

	fromCharts := make([]configv1beta1.Chart, 0)
	fromResources := make([]configv1beta1.Resource, 0)
	if fromClusterConfiguration != nil {
		for i := range fromClusterConfiguration.Status.ClusterProfileResources {
			cpr := &fromClusterConfiguration.Status.ClusterProfileResources[i]
			fromCharts, fromResources = appendChartsAndResourcesForClusterProfiles(cpr, fromCharts, fromResources)
		}
		for i := range fromClusterConfiguration.Status.ProfileResources {
			pr := &fromClusterConfiguration.Status.ProfileResources[i]
			fromCharts, fromResources = appendChartsAndResourcesForProfiles(pr, fromCharts, fromResources)
		}
	}

	// Evaluate the diff
	chartAdded, chartModified, chartDeleted, modifiedChartMessage :=
		chartDifference(fromCharts, toCharts)
	resourceAdded, resourceModified, resourceDeleted, err :=
		resourceDifference(fromFolder, toFolder, fromResources, toResources, rawDiff, logger)
	if err != nil {
		return err
	}

	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartAdded, "added", nil, table)
	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartModified, "modified", modifiedChartMessage, table)
	addChartEntry(fromClusterConfiguration, toClusterConfiguration, chartDeleted, "deleted", nil, table)

	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceAdded, "added", "", table)
	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceModified, "modified",
		"use --raw-diff option to see diff", table)
	addResourceEntry(fromClusterConfiguration, toClusterConfiguration, resourceDeleted, "deleted", "", table)
	return nil
}

func addChartEntry(fromClusterConfiguration, toClusterConfiguration *configv1beta1.ClusterConfiguration,
	charts []*configv1beta1.Chart, action string, message map[configv1beta1.Chart]string, table *tablewriter.Table) {

	instance := utils.GetAccessInstance()
	clusterInfo := func(fromClusterConfiguration, toClusterConfiguration *configv1beta1.ClusterConfiguration) string {
		if toClusterConfiguration != nil {
			clusterName := instance.GetClusterNameFromClusterConfiguration(toClusterConfiguration)
			return fmt.Sprintf("%s/%s", toClusterConfiguration.Namespace, clusterName)
		}
		clusterName := instance.GetClusterNameFromClusterConfiguration(fromClusterConfiguration)
		return fmt.Sprintf("%s/%s", fromClusterConfiguration.Namespace, clusterName)
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

func addResourceEntry(fromClusterConfiguration, toClusterConfiguration *configv1beta1.ClusterConfiguration,
	resources []*configv1beta1.Resource, action, msg string,
	table *tablewriter.Table) {

	clusterInfo := func(fromClusterConfiguration, toClusterConfiguration *configv1beta1.ClusterConfiguration) string {
		if toClusterConfiguration != nil {
			return fmt.Sprintf("%s/%s", toClusterConfiguration.Namespace, toClusterConfiguration.Name)
		}
		return fmt.Sprintf("%s/%s", fromClusterConfiguration.Namespace, fromClusterConfiguration.Name)
	}

	for i := range resources {
		table.Append(genSnapshotDiffRow(
			clusterInfo(fromClusterConfiguration, toClusterConfiguration),
			fmt.Sprintf("%s/%s", resources[i].Group, resources[i].Kind),
			resources[i].Namespace, resources[i].Name,
			action, msg))
	}
}

// chartDifference returns differences between from and to
func chartDifference(from, to []configv1beta1.Chart) (added, modified, deleted []*configv1beta1.Chart,
	modifiedMessage map[configv1beta1.Chart]string) {

	modifiedMessage = make(map[configv1beta1.Chart]string)

	chartInfo := func(chart *configv1beta1.Chart) string {
		return fmt.Sprintf("%s/%s", chart.Namespace, chart.ReleaseName)
	}

	toChartMap := make(map[string]*configv1beta1.Chart, len(to))
	for i := range to {
		chart := &to[i]
		toChartMap[chartInfo(chart)] = chart
	}

	fromChartMap := make(map[string]*configv1beta1.Chart, len(from))
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
func resourceDifference(fromFolder, toFolder string, from, to []configv1beta1.Resource, rawDiff bool,
	logger logr.Logger) (added, modified, deleted []*configv1beta1.Resource, err error) {

	resourceInfo := func(resource *configv1beta1.Resource) string {
		return fmt.Sprintf("%s:%s:%s/%s", resource.Group, resource.Kind, resource.Namespace, resource.Name)
	}

	toResourceMap := make(map[string]*configv1beta1.Resource, len(to))
	for i := range to {
		resource := &to[i]
		toResourceMap[resourceInfo(resource)] = resource
	}

	fromResourceMap := make(map[string]*configv1beta1.Resource, len(from))
	for i := range from {
		resource := &from[i]
		fromResourceMap[resourceInfo(resource)] = resource
	}

	var addedResources []*configv1beta1.Resource
	var deletedResources []*configv1beta1.Resource
	var modifiedResources []*configv1beta1.Resource

	for k := range toResourceMap {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("analyzing resource %s/%s %s/%s",
			toResourceMap[k].Group, toResourceMap[k].Kind, toResourceMap[k].Namespace, toResourceMap[k].Name))
		v, ok := fromResourceMap[k]
		if !ok {
			addedResources = append(addedResources, toResourceMap[k])
		} else if !reflect.DeepEqual(*v, *(toResourceMap[k])) {
			var diff bool
			diff, err = hasDiff(fromFolder, toFolder, v, toResourceMap[k], rawDiff)
			if err != nil {
				return nil, nil, nil, err
			}
			if diff {
				modifiedResources = append(modifiedResources, toResourceMap[k])
			}
		}
	}

	for k := range fromResourceMap {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("analyzing resource %s/%s %s/%s",
			fromResourceMap[k].Group, fromResourceMap[k].Kind, fromResourceMap[k].Namespace, fromResourceMap[k].Name))
		_, ok := toResourceMap[k]
		if !ok {
			deletedResources = append(deletedResources, fromResourceMap[k])
		}
	}

	return addedResources, modifiedResources, deletedResources, nil
}

func appendChartsAndResourcesForClusterProfiles(cpr *configv1beta1.ClusterProfileResource, charts []configv1beta1.Chart,
	resources []configv1beta1.Resource) ([]configv1beta1.Chart, []configv1beta1.Resource) {

	for i := range cpr.Features {
		for j := range cpr.Features[i].Charts {
			c := cpr.Features[i].Charts[j]
			c.LastAppliedTime = nil
			charts = append(charts, c)
		}
		resources = append(resources, cpr.Features[i].Resources...)
	}

	return charts, resources
}

func appendChartsAndResourcesForProfiles(pr *configv1beta1.ProfileResource, charts []configv1beta1.Chart,
	resources []configv1beta1.Resource) ([]configv1beta1.Chart, []configv1beta1.Resource) {

	for i := range pr.Features {
		for j := range pr.Features[i].Charts {
			c := pr.Features[i].Charts[j]
			c.LastAppliedTime = nil
			charts = append(charts, c)
		}
		resources = append(resources, pr.Features[i].Resources...)
	}

	return charts, resources
}

// hasDiff returns true if any diff exist
func hasDiff(fromFolder, toFolder string, from, to *configv1beta1.Resource, rawDiff bool) (bool, error) {
	fromResource, err := getResourceFromResourceOwner(fromFolder, from)
	if err != nil {
		return false, err
	}

	toResource, err := getResourceFromResourceOwner(toFolder, to)
	if err != nil {
		return false, err
	}

	objectInfo := fmt.Sprintf("%s %s/%s", from.Kind, from.Namespace, from.Name)
	edits := myers.ComputeEdits(span.URIFromPath(objectInfo), fromResource, toResource)
	resourceInfo := fmt.Sprintf("%s/%s ", from.Group, from.Kind)
	if from.Namespace == "" {
		resourceInfo += from.Name
	} else {
		resourceInfo += fmt.Sprintf("%s/%s", from.Namespace, from.Name)
	}
	diff := fmt.Sprint(gotextdiff.ToUnified(fmt.Sprintf("%s from %s", resourceInfo, fromFolder),
		fmt.Sprintf("%s from %s", resourceInfo, toFolder),
		fromResource, edits))

	if rawDiff && diff != "" {
		//nolint: forbidigo // print diff
		fmt.Println(diff)
	}

	return diff != "", nil
}

// hasResourceDiff returns true if any diff exist between two unstructured spec
func hasResourceDiff(from, to *unstructured.Unstructured, rawDiff bool, logger logr.Logger) (bool, error) {
	var fromContent = from.UnstructuredContent()

	var toContent = to.UnstructuredContent()

	const spec = "spec"
	if !reflect.DeepEqual(fromContent[spec], toContent[spec]) {
		objectInfo := fmt.Sprintf("%s %s", from.GroupVersionKind().Kind, from.GetName())

		fromJSON, err := json.MarshalIndent(fromContent[spec], "", "  ")
		if err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to convert %s to JSON. Err: %v",
				from.GroupVersionKind().Kind, err))
			return false, err
		}

		toJSON, err := json.MarshalIndent(toContent[spec], "", "  ")
		if err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to convert %s to JSON. Err: %v",
				from.GroupVersionKind().Kind, err))
			return false, err
		}

		edits := myers.ComputeEdits(span.URIFromPath(objectInfo), string(fromJSON), string(toJSON))
		diff := fmt.Sprint(gotextdiff.ToUnified(fmt.Sprintf("%s %s", from.GroupVersionKind().Kind, from.GetName()),
			fmt.Sprintf("%s %s", to.GroupVersionKind().Kind, to.GetName()),
			string(fromJSON), edits))

		if rawDiff {
			//nolint: forbidigo // print diff
			fmt.Println(diff)
		}

		return true, nil
	}
	return false, nil
}

func getResourceFromResourceOwner(folder string, resource *configv1beta1.Resource) (string, error) {
	ownerPath := buildOwnerPath(folder, resource)

	owner, err := getResourceOwner(ownerPath)
	if err != nil {
		return "", err
	}

	var data map[string]string
	if owner.DeepCopy().GroupVersionKind().Kind == string(libsveltosv1beta1.ConfigMapReferencedResourceKind) {
		var configMap corev1.ConfigMap
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(owner.UnstructuredContent(), &configMap)
		if err != nil {
			return "", err
		}
		data = configMap.Data
	} else {
		var secret corev1.Secret
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(owner.UnstructuredContent(), &secret)
		if err != nil {
			return "", err
		}
		data = make(map[string]string)
		for key, value := range secret.Data {
			data[key] = string(value)
		}
	}

	for k := range data {
		elements := strings.Split(data[k], "---")
		for i := range elements {
			if elements[i] == "" {
				continue
			}

			// We cannot get unstructured for the policy
			// as that will require scheme knows about it
			// and we don't know what type of policies are contained
			// in each ConfigMap/Secret. So just try to find string
			if strings.Contains(elements[i], resource.Group) &&
				strings.Contains(elements[i], resource.Kind) &&
				strings.Contains(elements[i], resource.Namespace) &&
				strings.Contains(elements[i], resource.Name) {

				return elements[i], nil
			}
		}
	}

	return "", fmt.Errorf("resource %s %s/%s not found in %s",
		resource.Kind, resource.Namespace, resource.Name, ownerPath)
}

func buildOwnerPath(folder string, resource *configv1beta1.Resource) string {
	return filepath.Join(folder,
		resource.Owner.Namespace,
		resource.Owner.Kind,
		fmt.Sprintf("%s.yaml", resource.Owner.Name))
}

func getResourceOwner(ownerFile string) (*unstructured.Unstructured, error) {
	content, err := os.ReadFile(ownerFile)
	if err != nil {
		return nil, err
	}

	instance := collector.GetClient()
	u, err := instance.GetUnstructured(content)
	if err != nil {
		return nil, err
	}

	return u, nil
}

// Diff lists differences between two snapshots
func Diff(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
	sveltosctl snapshot diff [options] --snapshot=<name> --from-sample=<name> --to-sample=<name> [--namespace=<name>] [--raw-diff] [--cluster=<name>] [--verbose]

     --snapshot=<name>      Name of the Snapshot instance
     --from-sample=<name>   Name of the directory containing this sample.
                            Use sveltosctl snapshot list to see all collected snapshosts.
     --to-sample=<name>     Name of the directory containing this sample.
                            Use sveltosctl snapshot list to see all collected snapshosts.
     --namespace=<name>     Show features differences for clusters in this namespace.
                            If not specified all namespaces are considered.
     --cluster=<name>       Show features differences for clusters with name.
                            If not specified all cluster names are considered.
     --raw-diff             With this flag, for each referenced ConfigMap/Secret, diff will be displayed.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The snapshot diff command list differences in deployed features in sample-two having sample-one as starting point
`

	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogDebug).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug))
		if err != nil {
			return err
		}
	}

	logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))

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

	rawDiff := parsedArgs["--raw-diff"].(bool)

	return listSnapshotDiffs(ctx, snapshostName, fromSample, toSample, namespace, cluster, rawDiff, logger)
}
