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
	"github.com/projectsveltos/sveltos-manager/controllers"
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

func displayDryRun(ctx context.Context, passedNamespace, passedCluster, passedClusterProfile string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"CLUSTER", "RESOURCE TYPE", "NAMESPACE", "NAME", "ACTION", "MESSAGE", "CLUSTER PROFILE"})

	if err := displayDryRunInNamespaces(ctx, passedNamespace, passedCluster,
		passedClusterProfile, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displayDryRunInNamespaces(ctx context.Context, passedNamespace, passedCluster, passedClusterProfile string,
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
			err = displayDryRunInNamespace(ctx, ns.Name, passedCluster, passedClusterProfile,
				table, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func displayDryRunInNamespace(ctx context.Context, namespace, passedCluster, passedClusterProfile string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger = logger.WithValues("namespace", namespace)
	logger.V(logs.LogDebug).Info("Get all ClusterReports")
	clusterReports, err := instance.ListClusterReports(ctx, namespace, logger)
	if err != nil {
		return err
	}

	instance.SortClusterReports(clusterReports.Items)

	for i := range clusterReports.Items {
		cc := &clusterReports.Items[i]
		clusterProfileLabel := cc.Labels[controllers.ClusterProfileLabelName]
		if doConsiderClusterReport(cc, passedCluster) &&
			doConsiderClusterProfile([]string{clusterProfileLabel}, passedClusterProfile) {

			logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering ClusterReport: %s", cc.Name))
			displayDryRunForCluster(cc, table)
		}
	}

	return nil
}

func displayDryRunForCluster(clusterReport *configv1alpha1.ClusterReport, table *tablewriter.Table) {
	clusterProfileName := clusterReport.Labels[controllers.ClusterProfileLabelName]
	clusterInfo := fmt.Sprintf("%s/%s", clusterReport.Spec.ClusterNamespace, clusterReport.Spec.ClusterName)
	for i := range clusterReport.Status.ReleaseReports {
		report := &clusterReport.Status.ReleaseReports[i]
		table.Append(genDryRunRow(clusterInfo, "helm release", report.ReleaseNamespace, report.ReleaseName,
			report.Action, report.Message, clusterProfileName))
	}

	for i := range clusterReport.Status.ResourceReports {
		report := &clusterReport.Status.ResourceReports[i]
		groupKind := fmt.Sprintf("%s:%s", report.Resource.Group, report.Resource.Kind)
		table.Append(genDryRunRow(clusterInfo, groupKind, report.Resource.Namespace, report.Resource.Name,
			report.Action, report.Message, clusterProfileName))
	}
}

// DryRun displays information about which features would change in which cluster due to a ClusterProfile currently in DryRun mode,
func DryRun(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show dryrun [options] [--namespace=<name>] [--cluster=<name>] [--clusterprofile=<name>] [--verbose]

     --namespace=<name>      Show which features would change in clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show which features would change in cluster with name.
                             If not specified all cluster names are considered.
     --clusterprofile=<name> Show which features would change because of this clusterprofile.
                             If not specified all clusterprofile names are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show dryrun command shows information about which features would change in a cluster due to ClusterProfiles in DryRun mode.
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

	return displayDryRun(ctx, namespace, cluster, clusterProfile, logger)
}
