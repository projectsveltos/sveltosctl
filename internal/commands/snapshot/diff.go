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
	"encoding/base64"
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
	"k8s.io/klog/v2/klogr"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
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

	genClassifierDiffRow = func(classifierName, action string) []string {
		return []string{
			classifierName,
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

	fromFolder := filepath.Join(*artifactFolder, fromSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(fromFolder)
	if os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			fromSample, snapshotName))
		return err
	}

	toFolder := filepath.Join(*artifactFolder, toSample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(toFolder)
	if os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			toSample, snapshotName))
		return err
	}

	err = listClusterResourcesDiff(fromFolder, toFolder, passedNamespace, passedCluster, rawDiff, logger)
	if err != nil {
		return err
	}

	err = listClassifiersDiff(fromFolder, toFolder, rawDiff, logger)
	if err != nil {
		return err
	}

	return nil
}

func listClassifiersDiff(fromFolder, toFolder string, rawDiff bool, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	fromClassifiers, err := snapshotClient.GetClusterResources(fromFolder, libsveltosv1alpha1.ClassifierKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect Classifiers from folder %s", fromFolder))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d Classifiers in folder %s", len(fromClassifiers), fromFolder))

	toClassifiers, err := snapshotClient.GetClusterResources(toFolder, libsveltosv1alpha1.ClassifierKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect Classifiers from folder %s", toFolder))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d Classifiers in folder %s", len(toClassifiers), toFolder))

	fromClassifierMap := make(map[string]*unstructured.Unstructured, len(fromClassifiers))
	for i := range fromClassifiers {
		cl := fromClassifiers[i]
		fromClassifierMap[cl.GetName()] = cl
	}

	toClassifierMap := make(map[string]*unstructured.Unstructured, len(toClassifiers))
	for i := range toClassifiers {
		cl := toClassifiers[i]
		toClassifierMap[cl.GetName()] = cl
	}

	err = showClassifiersDiff(fromClassifierMap, toClassifierMap, rawDiff, logger)
	if err != nil {
		return err
	}

	return nil
}

func showClassifiersDiff(fromClassifierMap, toClassifierMap map[string]*unstructured.Unstructured, rawDiff bool,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLASSIFIER", "ACTION"})

	for clName := range toClassifierMap {
		v, ok := fromClassifierMap[clName]
		if !ok {
			table.Append(genClassifierDiffRow(clName, "added"))
			continue
		}
		hasDiff, err := hasClassifierDiff(v, toClassifierMap[clName], rawDiff, logger)
		if err != nil {
			return err
		}
		if hasDiff {
			table.Append(genClassifierDiffRow(clName, "modified"))
		}
	}

	for clName := range fromClassifierMap {
		_, ok := toClassifierMap[clName]
		if !ok {
			table.Append(genClassifierDiffRow(clName, "removed"))
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
	snapshotClient := snapshotter.GetClient()
	fromClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(fromFolder,
		configv1alpha1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", fromFolder))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(fromClusterConfigurationMap), fromFolder))

	toClusterConfigurationMap, err := snapshotClient.GetNamespacedResources(toFolder,
		configv1alpha1.ClusterConfigurationKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterConfigurations from folder %s", toFolder))
		return err
	}
	logger.V(logs.LogVerbose).Info(fmt.Sprintf("found %d namespaces with at least one ClusterConfiguration in folder %s",
		len(toClusterConfigurationMap), fromFolder))

	err = listFeatureDiff(fromFolder, toFolder, fromClusterConfigurationMap, toClusterConfigurationMap,
		passedNamespace, passedCluster, rawDiff, table, logger)
	if err != nil {
		return nil
	}

	return nil
}

func listFeatureDiff(fromFolder, toFolder string,
	fromClusterConfigurationMap, toClusterConfigurationMap map[string][]*unstructured.Unstructured,
	passedNamespace, passedCluster string, rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

	for k := range toClusterConfigurationMap {
		if doConsiderNamespace(k, passedNamespace) {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("finding feature diff for clusters in namespace %s", k))
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
	err = listDiffInClusterConfigurations(fromFolder, toFolder, fromClusterConfigurations, toClusterConfigurations, rawDiff, table, logger)
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

func listDiffInClusterConfigurations(fromFolder, toFolder string, fromClusterConfigurations, toClusterConfigurations []*configv1alpha1.ClusterConfiguration,
	rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

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

func listClusterConfigurationDiff(fromFolder, toFolder string, fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration,
	rawDiff bool, table *tablewriter.Table, logger logr.Logger) error {

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
	resources []*configv1alpha1.Resource, action, msg string,
	table *tablewriter.Table) {

	clusterInfo := func(fromClusterConfiguration, toClusterConfiguration *configv1alpha1.ClusterConfiguration) string {
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
func resourceDifference(fromFolder, toFolder string, from, to []configv1alpha1.Resource, rawDiff bool,
	logger logr.Logger) (added, modified, deleted []*configv1alpha1.Resource, err error) {

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

	for k := range toResourceMap {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("analyzing resource %s/%s %s/%s",
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
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("analyzing resource %s/%s %s/%s",
			fromResourceMap[k].Group, fromResourceMap[k].Kind, fromResourceMap[k].Namespace, fromResourceMap[k].Name))
		_, ok := toResourceMap[k]
		if !ok {
			deletedResources = append(deletedResources, fromResourceMap[k])
		}
	}

	return addedResources, modifiedResources, deletedResources, nil
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

// hasDiff returns true if any diff exist
func hasDiff(fromFolder, toFolder string, from, to *configv1alpha1.Resource, rawDiff bool) (bool, error) {
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

	if rawDiff {
		//nolint: forbidigo // print diff
		fmt.Println(diff)
	}

	return diff != "", nil
}

// hasClassifierDiff returns true if any diff exist between two Classifiers
func hasClassifierDiff(from, to *unstructured.Unstructured, rawDiff bool, logger logr.Logger) (bool, error) {
	var fromClassifier libsveltosv1alpha1.Classifier
	err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(from.UnstructuredContent(), &fromClassifier)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to convert unstructured to Classifier. Err: %v", err))
		return false, err
	}

	var toClassifier libsveltosv1alpha1.Classifier
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(to.UnstructuredContent(), &toClassifier)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to convert unstructured to Classifier. Err: %v", err))
		return false, err
	}
	if !reflect.DeepEqual(fromClassifier.Spec, toClassifier.Spec) {
		objectInfo := fmt.Sprintf("%s %s", fromClassifier.Kind, fromClassifier.Name)

		fromJSON, err := json.MarshalIndent(fromClassifier.Spec, "", "  ")
		if err != nil {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to convert Classifier to JSON. Err: %v", err))
			return false, err
		}

		toJSON, err := json.MarshalIndent(toClassifier.Spec, "", "  ")
		if err != nil {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to convert Classifier to JSON. Err: %v", err))
			return false, err
		}

		edits := myers.ComputeEdits(span.URIFromPath(objectInfo), string(fromJSON), string(toJSON))
		diff := fmt.Sprint(gotextdiff.ToUnified(fmt.Sprintf("Classifier %s", fromClassifier.Name),
			fmt.Sprintf("Classifier %s", toClassifier.Name),
			string(fromJSON), edits))

		if rawDiff {
			//nolint: forbidigo // print diff
			fmt.Println(diff)
		}

		return true, nil
	}
	return false, nil
}

func getResourceFromResourceOwner(folder string, resource *configv1alpha1.Resource) (string, error) {
	ownerPath := buildOwnerPath(folder, resource)

	owner, err := getResourceOwner(ownerPath)
	if err != nil {
		return "", err
	}

	var data map[string]string
	if owner.DeepCopy().GroupVersionKind().Kind == string(configv1alpha1.ConfigMapReferencedResourceKind) {
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
			data[key], err = decode(value)
			if err != nil {
				return "", err
			}
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

func decode(encoded []byte) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func buildOwnerPath(folder string, resource *configv1alpha1.Resource) string {
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

	instance := snapshotter.GetClient()
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

	rawDiff := parsedArgs["--raw-diff"].(bool)

	return listSnapshotDiffs(ctx, snapshostName, fromSample, toSample, namespace, cluster, rawDiff, logger)
}
