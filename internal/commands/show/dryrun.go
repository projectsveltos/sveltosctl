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
	"regexp"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// cluster represents the cluster => namespace/name
	// resourceNamespace and resourceName is the kubernetes resource/helm release namespace/name
	// action represents the type of action that would be take effect on the resource
	// clusterProfileNames is the list of all ClusterProfiles causing the resource to be deployed
	// in the cluster
	genDryRunRow = func(cluster, resourceType, resourceNamespace, resourceName, action, message, clusterProfileName string,
	) []string {
		return []string{
			cluster,
			resourceType,
			resourceNamespace,
			resourceName,
			action,
			message,
			clusterProfileName,
		}
	}
)

func displayDryRun(ctx context.Context, passedNamespace, passedCluster, passedProfile string,
	rawDiff bool, logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "ACTION", "MESSAGE", "PROFILE"})

	if err := displayDryRunInNamespaces(ctx, passedNamespace, passedCluster,
		passedProfile, table, rawDiff, logger); err != nil {
		return err
	}

	if !rawDiff {
		table.Render()
	}

	return nil
}

func displayDryRunInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedProfile string,
	table *tablewriter.Table, rawDiff bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	namespaces, err := instance.ListNamespaces(ctx, logger)
	if err != nil {
		return err
	}

	for i := range namespaces.Items {
		ns := &namespaces.Items[i]
		if doConsiderNamespace(ns, passedNamespace) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering namespace: %s", ns.Name))
			err = displayDryRunInNamespace(ctx, ns.Name, passedCluster, passedProfile,
				table, rawDiff, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayDryRunInNamespace(ctx context.Context, namespace, passedCluster, passedProfile string,
	table *tablewriter.Table, rawDiff bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger = logger.WithValues("namespace", namespace)
	logger.V(logs.LogDebug).Info("Get all ClusterReports")
	clusterReports, err := instance.ListClusterReports(ctx, namespace, logger)
	if err != nil {
		return err
	}

	instance.SortClusterReports(clusterReports.Items)

	pattern := regexp.MustCompile("p--(.*)")

	for i := range clusterReports.Items {
		cr := &clusterReports.Items[i]
		profileLabel := cr.Labels["projectsveltos.io/cluster-profile-name"]

		// TODO: find a better way to identify clusterreports created by ClusterProfile
		// vs clusterreports created by Profile
		var profileName string
		// Create a regular expression pattern to match strings that start with "p--"
		match := pattern.MatchString(cr.Name)
		if match {
			profileName = fmt.Sprintf("Profile/%s", profileLabel)
		} else {
			profileName = fmt.Sprintf("ClusterProfile/%s", profileLabel)
		}

		if doConsiderClusterReport(cr, passedCluster) &&
			doConsiderProfile([]string{profileName}, passedProfile) {

			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering ClusterReport: %s", cr.Name))
			err = displayDryRunForCluster(cr, profileName, table, rawDiff)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayDryRunForCluster(clusterReport *configv1beta1.ClusterReport, profileName string,
	table *tablewriter.Table, rawDiff bool) error {

	clusterInfo := fmt.Sprintf("%s/%s", clusterReport.Spec.ClusterNamespace, clusterReport.Spec.ClusterName)
	profileOwner, err := getProfileOwnerReference(clusterReport)
	if err != nil {
		return err
	}

	updateMessage := "use --raw-diff to see full diff for helm values"
	for i := range clusterReport.Status.ReleaseReports {
		report := &clusterReport.Status.ReleaseReports[i]
		message := report.Message
		if report.Action == string(configv1beta1.UpdateHelmValuesAction) {
			message = updateMessage
		}
		table.Append(genDryRunRow(clusterInfo, "helm release", report.ReleaseNamespace, report.ReleaseName,
			report.Action, message, profileName))
		if rawDiff {
			if rawDiff && report.Message != "" && report.Action == string(configv1beta1.UpdateHelmValuesAction) {
				//nolint: forbidigo // print diff
				fmt.Printf("Profile: %s:%s Cluster: %s/%s\n%s\n", profileOwner.Kind, profileOwner.Name,
					clusterReport.Spec.ClusterNamespace, clusterReport.Spec.ClusterName, report.Message)
			}
		}
	}

	updateMessage = "use --raw-diff to see full diff"
	for i := range clusterReport.Status.ResourceReports {
		report := &clusterReport.Status.ResourceReports[i]
		groupKind := fmt.Sprintf("%s:%s", report.Resource.Group, report.Resource.Kind)
		message := report.Message
		if report.Action == string(configv1beta1.UpdateResourceAction) {
			message = updateMessage
		}
		table.Append(genDryRunRow(clusterInfo, groupKind, report.Resource.Namespace, report.Resource.Name,
			report.Action, message, profileName))
		if rawDiff {
			if rawDiff && report.Message != "" && report.Action == string(configv1beta1.UpdateResourceAction) {
				//nolint: forbidigo // print diff
				fmt.Printf("Profile: %s:%s Cluster: %s/%s\n%s\n", profileOwner.Kind, profileOwner.Name,
					clusterReport.Spec.ClusterNamespace, clusterReport.Spec.ClusterName, report.Message)
			}
		}
	}

	for i := range clusterReport.Status.KustomizeResourceReports {
		report := &clusterReport.Status.KustomizeResourceReports[i]
		groupKind := fmt.Sprintf("%s:%s", report.Resource.Group, report.Resource.Kind)
		message := report.Message
		if report.Action == string(configv1beta1.UpdateResourceAction) {
			message = updateMessage
		}
		table.Append(genDryRunRow(clusterInfo, groupKind, report.Resource.Namespace, report.Resource.Name,
			report.Action, message, profileName))
		if rawDiff {
			if rawDiff && report.Message != "" && report.Action == string(configv1beta1.UpdateResourceAction) {
				//nolint: forbidigo // print diff
				fmt.Printf("Profile: %s:%s Cluster: %s/%s\n%s\n", profileOwner.Kind, profileOwner.Name,
					clusterReport.Spec.ClusterNamespace, clusterReport.Spec.ClusterName, report.Message)
			}
		}
	}

	return nil
}

// DryRun displays information about which Kubernetes addons would change in which cluster due
// to a ClusterProfile currently in DryRun mode,
func DryRun(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show dryrun [options] [--namespace=<name>] [--cluster=<name>] [--profile=<name>] [--raw-diff] [--verbose]

     --namespace=<name>      Show which Kubernetes addons would change in clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show which Kubernetes addons would change in cluster with name.
                             If not specified all cluster names are considered.
     --profile=<kind/name>   Show which Kubernetes addons would change because of this clusterprofile/profile.
                             If not specified all clusterprofiles/profiles are considered.
     --raw-diff              With this flag, for each resource that would be update, full diff will be displayed.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show dryrun command shows information about which Kubernetes addons would change in a cluster due to ClusterProfiles in DryRun mode.
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

	rawDiff := parsedArgs["--raw-diff"].(bool)

	return displayDryRun(ctx, namespace, cluster, profile, rawDiff, logger)
}

// getProfileOwnerReference returns the ClusterProfile/Profile owning a given ClusterReport
func getProfileOwnerReference(clusterReport *configv1beta1.ClusterReport) (*metav1.OwnerReference, error) {
	for _, ref := range clusterReport.OwnerReferences {
		if ref.Kind != configv1beta1.ClusterProfileKind &&
			ref.Kind != configv1beta1.ProfileKind {

			continue
		}
		gv, err := schema.ParseGroupVersion(ref.APIVersion)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		if gv.Group == configv1beta1.GroupVersion.Group {
			return &ref, nil
		}
	}

	return nil, fmt.Errorf("(Cluster)Profile owner not found")
}
