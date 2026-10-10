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
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	// noValue is displayed in a column which does not apply to a drifted resource
	noValue = "-"

	// driftNotReported is displayed in place of the resource when a drift was detected
	// but the component that detected it did not report which resources changed
	driftNotReported = "(not reported)"
)

// DriftHistoryFilter selects which drift histories are displayed. An empty field matches everything.
type DriftHistoryFilter struct {
	Namespace   string
	Cluster     string
	ClusterType string
	Profile     string
	Feature     string
}

var (
	// cluster represents the cluster => namespace/name
	// profile represents the profile => kind/name
	// feature is the feature (Helm, Resources, Kustomize) which deployed the resource
	// kind is the drifted resource kind, in kubectl notation (Kind.group)
	// namespace and name identify the drifted resource
	// detectedTime is when the latest drift of the resource was detected (RFC3339, UTC)
	// helmRelease is the helm release namespace/name which deployed the resource (Helm feature only)
	genDriftHistoryRow = func(cluster, profile, feature, kind, namespace, name, detectedTime, helmRelease string) []string {
		return []string{
			cluster,
			profile,
			feature,
			kind,
			namespace,
			name,
			detectedTime,
			helmRelease,
		}
	}
)

func displayDriftHistory(ctx context.Context, filter *DriftHistoryFilter, logger logr.Logger) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "PROFILE", "FEATURE", "KIND", "NAMESPACE", "NAME", "DETECTED", "HELM RELEASE")

	instance := utils.GetAccessInstance()

	clusterSummaries, err := instance.ListClusterSummaries(ctx, filter.Namespace, filter.Cluster,
		filter.ClusterType, logger)
	if err != nil {
		return err
	}

	truncatedNotes := []string{}
	for i := range clusterSummaries.Items {
		clusterSummary := &clusterSummaries.Items[i]

		ownerRef, err := configv1beta1.GetProfileOwnerReference(clusterSummary)
		if err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Skipping ClusterSummary %s/%s: %v",
				clusterSummary.Namespace, clusterSummary.Name, err))
			continue
		}
		if filter.Profile != "" && ownerRef.Name != filter.Profile {
			continue
		}

		profile := fmt.Sprintf("%s/%s", ownerRef.Kind, ownerRef.Name)
		notes, err := appendDriftHistoryRows(clusterSummary, profile, filter.Feature, table)
		if err != nil {
			return err
		}
		truncatedNotes = append(truncatedNotes, notes...)
	}

	if err := table.Render(); err != nil {
		return err
	}

	for i := range truncatedNotes {
		//nolint: forbidigo // print the note under the table
		fmt.Println(truncatedNotes[i])
	}

	return nil
}

// appendDriftHistoryRows adds to the table one row for each resource which drifted in the
// ClusterSummary. It returns a note for each feature whose history is truncated.
func appendDriftHistoryRows(clusterSummary *configv1beta1.ClusterSummary, profile, feature string,
	table *tablewriter.Table) ([]string, error) {

	clusterInfo := fmt.Sprintf("%s/%s", clusterSummary.Spec.ClusterNamespace, clusterSummary.Spec.ClusterName)

	notes := []string{}
	for i := range clusterSummary.Status.FeatureSummaries {
		featureSummary := &clusterSummary.Status.FeatureSummaries[i]
		featureID := string(featureSummary.FeatureID)

		if featureSummary.DriftHistory == nil ||
			(feature != "" && !strings.EqualFold(feature, featureID)) {

			continue
		}

		history := featureSummary.DriftHistory

		if len(history.Resources) == 0 {
			if err := table.Append(genDriftHistoryRow(clusterInfo, profile, featureID,
				driftNotReported, noValue, noValue, formatDetectedTime(&history.LastDetectedTime), noValue)); err != nil {
				return nil, err
			}
		}

		for j := range history.Resources {
			resource := &history.Resources[j]
			if err := table.Append(genDriftHistoryRow(clusterInfo, profile, featureID,
				formatDriftedKind(resource), valueOrNone(resource.Namespace), resource.Name,
				formatDetectedTime(&resource.DetectedTime), formatHelmRelease(resource))); err != nil {
				return nil, err
			}
		}

		if history.Truncated {
			notes = append(notes, fmt.Sprintf(
				"Note: %s %s %s: the latest drift involved more resources than could be reported, so the list is incomplete.",
				clusterInfo, profile, featureID))
		}
	}

	return notes, nil
}

// formatDriftedKind returns the resource kind in kubectl notation: Kind.group, or Kind for the core group
func formatDriftedKind(resource *configv1beta1.DriftedResourceRef) string {
	if resource.Group == "" {
		return resource.Kind
	}

	return fmt.Sprintf("%s.%s", resource.Kind, resource.Group)
}

func formatHelmRelease(resource *configv1beta1.DriftedResourceRef) string {
	if resource.HelmReleaseName == "" {
		return noValue
	}

	if resource.HelmReleaseNamespace == "" {
		return resource.HelmReleaseName
	}

	return fmt.Sprintf("%s/%s", resource.HelmReleaseNamespace, resource.HelmReleaseName)
}

func formatDetectedTime(detectedTime *metav1.Time) string {
	return detectedTime.UTC().Format(time.RFC3339)
}

func valueOrNone(value string) string {
	if value == "" {
		return noValue
	}

	return value
}

// isValidDriftFeature returns true if feature is a feature for which drift is detected (case insensitive)
func isValidDriftFeature(feature string) bool {
	for _, validFeature := range []libsveltosv1beta1.FeatureID{
		libsveltosv1beta1.FeatureHelm,
		libsveltosv1beta1.FeatureResources,
		libsveltosv1beta1.FeatureKustomize,
	} {
		if strings.EqualFold(feature, string(validFeature)) {
			return true
		}
	}

	return false
}

// DriftHistory displays the resources that drifted from the configuration Sveltos deployed,
// and when the drift was detected
func DriftHistory(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show drift-history [options] [--namespace=<name>] [--cluster=<name>] [--cluster-type=<type>] [--profile=<name>] [--feature=<name>] [--verbose]

     --namespace=<name>      Show drift history of clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show drift history of cluster with name.
                             If not specified all cluster names are considered.
     --cluster-type=<type>   Show drift history of clusters with this type
                             (Capi or Sveltos). If not specified all cluster types are considered.
     --profile=<name>        Show drift history of resources deployed by the (Cluster)Profile with this name.
                             If not specified all profiles are considered.
     --feature=<name>        Show drift history of resources deployed by this feature
                             (Helm, Resources or Kustomize). If not specified all features are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The show drift-history command shows the resources that drifted from the configuration
  Sveltos deployed, and when each drift was detected. Drift is only detected for profiles
  with syncMode ContinuousWithDriftDetection.
  For each feature the most recently drifted resources are displayed first. A resource that
  drifts again is displayed once, with the time of its latest drift. Times are RFC3339, in UTC.
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

	filter := &DriftHistoryFilter{}
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
	if passedFeature := parsedArgs["--feature"]; passedFeature != nil {
		filter.Feature = passedFeature.(string)
		if !isValidDriftFeature(filter.Feature) {
			return fmt.Errorf("invalid feature %q: must be one of Helm, Resources, Kustomize", filter.Feature)
		}
	}

	return displayDriftHistory(ctx, filter, logger)
}
