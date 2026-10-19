/*
Copyright 2026. projectsveltos.io. All rights reserved.

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
	"sort"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	"github.com/projectsveltos/addon-controller/lib/healthmonitoring"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// UnhealthyResourcesFilter selects which unhealthy resources are displayed. An empty field matches everything.
type UnhealthyResourcesFilter struct {
	Namespace   string
	Cluster     string
	ClusterType string
	Profile     string
}

// clusterKey identifies a cluster
type clusterKey struct {
	namespace   string
	name        string
	clusterType libsveltosv1beta1.ClusterType
}

func (k clusterKey) String() string {
	return fmt.Sprintf("%s/%s", k.namespace, k.name)
}

// clusterHealthInput is what the health evaluation needs for one cluster
type clusterHealthInput struct {
	summaries []healthmonitoring.SummaryInput
	reports   []healthmonitoring.ReportInput
}

var (
	// cluster represents the cluster => namespace/name
	// profile is the profile(s) owning the HealthCheck which reported the resource (kind/name), or
	// the HealthCheck (HealthCheck/name) when no profile owns it
	// kind is the resource kind, in kubectl notation (Kind.group)
	// namespace and name identify the resource
	// status is the health status of the resource
	genUnhealthyResourceRow = func(cluster, profile, kind, namespace, name, status, message string) []string {
		return []string{
			cluster,
			profile,
			kind,
			namespace,
			name,
			status,
			message,
		}
	}
)

func displayUnhealthyResources(ctx context.Context, filter *UnhealthyResourcesFilter,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "PROFILE", "KIND", "NAMESPACE", "NAME", "STATUS", "MESSAGE")

	inputs, err := collectClusterHealthInputs(ctx, filter, logger)
	if err != nil {
		return err
	}

	clusters := sortedClusterKeys(inputs)

	notes := []string{}
	for i := range clusters {
		cluster := clusters[i]
		input := inputs[cluster]

		health := healthmonitoring.Evaluate(input.summaries, input.reports)
		if err := appendUnhealthyResourceRows(cluster, &health, filter.Profile, table); err != nil {
			return err
		}
		notes = append(notes, healthStateNotes(cluster, input, &health, filter.Profile)...)
	}

	if err := table.Render(); err != nil {
		return err
	}

	for i := range notes {
		//nolint: forbidigo // print the note under the table
		fmt.Println(notes[i])
	}

	return nil
}

// collectClusterHealthInputs lists ClusterSummaries, generated HealthChecks and HealthCheckReports and
// groups them by cluster. Only the clusters matching the filter are returned, and the selection is
// done by the API server: the cost does not grow with the number of clusters not selected. The profile
// filter is not applied here: the health of a cluster is evaluated considering all its profiles.
func collectClusterHealthInputs(ctx context.Context, filter *UnhealthyResourcesFilter,
	logger logr.Logger) (map[clusterKey]*clusterHealthInput, error) {

	instance := utils.GetAccessInstance()

	clusterSummaries, err := instance.ListClusterSummaries(ctx, filter.Namespace, filter.Cluster,
		filter.ClusterType, logger)
	if err != nil {
		return nil, err
	}

	generatedChecks, err := instance.ListGeneratedHealthChecks(ctx, healthMonitoredSummaryUIDs(clusterSummaries),
		logger)
	if err != nil {
		return nil, err
	}

	healthCheckReports, err := instance.ListClusterHealthCheckReports(ctx, filter.Namespace, filter.Cluster,
		filter.ClusterType, logger)
	if err != nil {
		return nil, err
	}

	inputs := map[clusterKey]*clusterHealthInput{}
	for i := range clusterSummaries.Items {
		clusterSummary := &clusterSummaries.Items[i]

		summaryInput, err := healthmonitoring.NewSummaryInput(clusterSummary,
			generatedChecks[types.NamespacedName{Namespace: clusterSummary.Namespace, Name: clusterSummary.Name}])
		if err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Skipping ClusterSummary %s/%s: %v",
				clusterSummary.Namespace, clusterSummary.Name, err))
			continue
		}

		input := getClusterHealthInput(inputs, clusterKey{
			namespace:   clusterSummary.Spec.ClusterNamespace,
			name:        clusterSummary.Spec.ClusterName,
			clusterType: clusterSummary.Spec.ClusterType,
		})
		input.summaries = append(input.summaries, summaryInput)
	}

	for i := range healthCheckReports.Items {
		report := &healthCheckReports.Items[i]
		input := getClusterHealthInput(inputs, clusterKey{
			namespace:   report.Spec.ClusterNamespace,
			name:        report.Spec.ClusterName,
			clusterType: report.Spec.ClusterType,
		})
		input.reports = append(input.reports, healthmonitoring.NewReportInput(report))
	}

	return inputs, nil
}

func getClusterHealthInput(inputs map[clusterKey]*clusterHealthInput, cluster clusterKey) *clusterHealthInput {
	if _, ok := inputs[cluster]; !ok {
		inputs[cluster] = &clusterHealthInput{}
	}

	return inputs[cluster]
}

// healthMonitoredSummaryUIDs returns the UIDs of the ClusterSummaries with health monitoring enabled:
// they are the only ones a HealthCheck is generated for.
func healthMonitoredSummaryUIDs(clusterSummaries *configv1beta1.ClusterSummaryList) []string {
	uids := []string{}
	for i := range clusterSummaries.Items {
		healthMonitoring := clusterSummaries.Items[i].Spec.ClusterProfileSpec.HealthMonitoring
		if healthMonitoring == nil || !healthMonitoring.Enabled {
			continue
		}
		uids = append(uids, string(clusterSummaries.Items[i].UID))
	}

	return uids
}

func sortedClusterKeys(inputs map[clusterKey]*clusterHealthInput) []clusterKey {
	clusters := make([]clusterKey, 0, len(inputs))
	for cluster := range inputs {
		clusters = append(clusters, cluster)
	}

	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].String() != clusters[j].String() {
			return clusters[i].String() < clusters[j].String()
		}
		return clusters[i].clusterType < clusters[j].clusterType
	})

	return clusters
}

// appendUnhealthyResourceRows adds to the table one row for each unhealthy resource of the cluster.
// When profile is not empty, only the resources reported through that profile are added.
func appendUnhealthyResourceRows(cluster clusterKey, health *healthmonitoring.ClusterHealth,
	profile string, table *tablewriter.Table) error {

	for i := range health.Resources {
		resource := &health.Resources[i]
		if !isOwnedByProfile(resource, profile) {
			continue
		}

		if err := table.Append(genUnhealthyResourceRow(cluster.String(), formatResourceOwners(resource),
			formatObjectKind(&resource.ObjectRef), valueOrNone(resource.ObjectRef.Namespace),
			resource.ObjectRef.Name, string(resource.Status), valueOrNone(resource.Message))); err != nil {
			return err
		}
	}

	return nil
}

func isOwnedByProfile(resource *healthmonitoring.UnhealthyResource, profile string) bool {
	if profile == "" {
		return true
	}

	for i := range resource.Profiles {
		if resource.Profiles[i].Name == profile {
			return true
		}
	}

	return false
}

// formatResourceOwners returns the profiles owning the HealthCheck which reported the resource
// (kind/name), or the HealthCheck when no profile owns it.
func formatResourceOwners(resource *healthmonitoring.UnhealthyResource) string {
	if len(resource.Profiles) == 0 {
		return fmt.Sprintf("HealthCheck/%s", resource.HealthCheckName)
	}

	owners := make([]string, len(resource.Profiles))
	for i := range resource.Profiles {
		owners[i] = fmt.Sprintf("%s/%s", resource.Profiles[i].Kind, resource.Profiles[i].Name)
	}

	return strings.Join(owners, ", ")
}

// formatObjectKind returns the resource kind in kubectl notation: Kind.group, or Kind for the core group
func formatObjectKind(ref *corev1.ObjectReference) string {
	gv, err := schema.ParseGroupVersion(ref.APIVersion)
	if err != nil || gv.Group == "" {
		return ref.Kind
	}

	return fmt.Sprintf("%s.%s", ref.Kind, gv.Group)
}

// healthStateNotes returns what to print under the table to explain an absence of rows, so that an
// empty table is never read as healthy. Without a profile filter it reports the state of the cluster,
// otherwise the state of the filtered profile on the cluster.
func healthStateNotes(cluster clusterKey, input *clusterHealthInput, health *healthmonitoring.ClusterHealth,
	profile string) []string {

	if profile == "" {
		return stateNote(cluster.String(), health.State)
	}

	notes := []string{}
	for i := range input.summaries {
		summary := &input.summaries[i]
		if summary.Profile.Name != profile {
			continue
		}

		subject := fmt.Sprintf("%s %s/%s", cluster, summary.Profile.Kind, summary.Profile.Name)
		notes = append(notes, stateNote(subject, profileState(summary, health))...)
	}

	return notes
}

// profileState returns the health state of the profile owning the ClusterSummary
func profileState(summary *healthmonitoring.SummaryInput, health *healthmonitoring.ClusterHealth) healthmonitoring.State {
	if !summary.Enabled {
		return healthmonitoring.StateNotEnabled
	}

	for i := range health.Profiles {
		if health.Profiles[i].Summary.Name == summary.Summary.Name &&
			health.Profiles[i].Summary.Namespace == summary.Summary.Namespace {

			return health.Profiles[i].State
		}
	}

	return healthmonitoring.StateNoData
}

// stateNote returns the note explaining a state with no unhealthy resources to display. States
// with unhealthy resources need no note.
func stateNote(subject string, state healthmonitoring.State) []string {
	switch state {
	case healthmonitoring.StateNotEnabled:
		return []string{fmt.Sprintf("Note: %s: health monitoring is not enabled.", subject)}
	case healthmonitoring.StateNoData:
		return []string{fmt.Sprintf("Note: %s: health monitoring is enabled but no health report has been received yet.", subject)}
	case healthmonitoring.StateHealthy:
		return []string{fmt.Sprintf("Note: %s: no unhealthy resources.", subject)}
	case healthmonitoring.StateSuspended, healthmonitoring.StateProgressing, healthmonitoring.StateDegraded:
		return nil
	}

	return nil
}

// UnhealthyResources shows the resources found not Healthy by health monitoring
func UnhealthyResources(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show unhealthy-resources [options] [--namespace=<name>] [--cluster=<name>] [--cluster-type=<type>] [--profile=<name>] [--verbose]

     --namespace=<name>      Show unhealthy resources of clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show unhealthy resources of cluster with name.
                             If not specified all cluster names are considered.
     --cluster-type=<type>   Show unhealthy resources of clusters with this type
                             (Capi or Sveltos). If not specified all cluster types are considered.
     --profile=<name>        Show only the unhealthy resources reported through the (Cluster)Profile with this name.
                             Resources no profile owns are not displayed. If not specified all profiles are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The show unhealthy-resources command shows the resources deployed by profiles with health monitoring
  enabled, and any other resource a HealthCheck reports, which are not Healthy.
  For each resource the profile owning the HealthCheck which reported it is displayed; when no profile
  owns the HealthCheck, the HealthCheck is displayed instead. Worst status first.
  When nothing is displayed for a cluster, a note explains why: health monitoring is not enabled,
  no health report has been received yet, or all resources are healthy.
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

	filter := &UnhealthyResourcesFilter{}
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		filter.Namespace = passedNamespace.(string)
	}
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		filter.Cluster = passedCluster.(string)
	}
	if passedClusterType := parsedArgs["--cluster-type"]; passedClusterType != nil {
		filter.ClusterType = passedClusterType.(string)
	}
	if passedProfile := parsedArgs["--profile"]; passedProfile != nil {
		filter.Profile = passedProfile.(string)
	}

	return displayUnhealthyResources(ctx, filter, logger)
}
