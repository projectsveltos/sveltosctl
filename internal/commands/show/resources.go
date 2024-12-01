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
	"github.com/fatih/color"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v3"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/k8s_utils"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

var (
	// cluster represents the cluster => namespace/name
	// gvk represents the resource group/version/kind
	// resourceNamespace and resourceName is the kubernetes resource namespace/name
	genResourceRow = func(cluster, resourceGVK, resourceNamespace, resourceName, message string) []string {
		return []string{
			cluster,
			resourceGVK,
			resourceNamespace,
			resourceName,
			message,
		}
	}
)

func displayResources(ctx context.Context,
	passedClusterNamespace, passedCluster, passedGroup, passedKind, passedNamespace string,
	full bool, logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetBorder(true)

	if !full {
		table.SetHeader([]string{"CLUSTER", "GVK", "NAMESPACE", "NAME", "MESSAGE"})
		table.SetAutoMergeCellsByColumnIndex([]int{0, 1})
		table.SetColumnColor(tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlackColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlackColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlackColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlackColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlackColor})
	}

	if err := displayResourcesInNamespaces(ctx, passedClusterNamespace, passedCluster,
		passedGroup, passedKind, passedNamespace, full, table, logger); err != nil {
		return err
	}

	if !full {
		table.Render()
	}

	return nil
}

func displayResourcesInNamespaces(ctx context.Context,
	passedClusterNamespace, passedCluster, passedGroup, passedKind, passedNamespace string,
	full bool, table *tablewriter.Table, logger logr.Logger) error {

	instance := k8s_utils.GetAccessInstance()

	healthCheckReports, err := instance.ListHealthCheckReports(ctx, passedClusterNamespace, logger)
	if err != nil {
		return err
	}

	for i := range healthCheckReports.Items {
		hcr := &healthCheckReports.Items[i]
		if passedCluster != "" && hcr.Spec.ClusterName != passedCluster {
			continue
		}
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering healthCheckReport: %s/%s",
			hcr.Namespace, hcr.Name))
		err = displayResourcesInReport(hcr, passedGroup, passedKind, passedNamespace,
			full, table, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func displayResourcesInReport(healthCheckReport *libsveltosv1beta1.HealthCheckReport,
	passedGroup, passedKind, passedNamespace string, full bool,
	table *tablewriter.Table, logger logr.Logger) error {

	logger = logger.WithValues("healtcheckreport", fmt.Sprintf("%s/%s",
		healthCheckReport.Namespace, healthCheckReport.Name))

	for i := range healthCheckReport.Spec.ResourceStatuses {
		resourceStatus := &healthCheckReport.Spec.ResourceStatuses[i]
		if doConsiderResourceStatus(resourceStatus, passedGroup, passedKind, passedNamespace) {
			logger.V(logs.LogDebug).Info("Considering resources in healthCheckReport")
			if full {
				err := printResource(resourceStatus, healthCheckReport.Spec.ClusterNamespace,
					healthCheckReport.Spec.ClusterName, logger)
				if err != nil {
					return err
				}
			} else {
				displayResource(resourceStatus, healthCheckReport.Spec.ClusterNamespace,
					healthCheckReport.Spec.ClusterName, table)
			}
		}
	}

	return nil
}

func displayResource(resourceStatus *libsveltosv1beta1.ResourceStatus,
	clusterNamespace, clusterName string, table *tablewriter.Table) {

	clusterInfo := fmt.Sprintf("%s/%s", clusterNamespace, clusterName)
	gvk := resourceStatus.ObjectRef.GroupVersionKind().String()
	resourceNamespace := resourceStatus.ObjectRef.Namespace
	resourceName := resourceStatus.ObjectRef.Name
	message := resourceStatus.Message

	if resourceStatus.HealthStatus != libsveltosv1beta1.HealthStatusHealthy {
		data := []string{clusterInfo, gvk, resourceNamespace, resourceName, message}
		table.Rich(data, []tablewriter.Colors{{tablewriter.Bold, tablewriter.FgBlackColor},
			{tablewriter.Bold, tablewriter.FgBlackColor}, {tablewriter.Bold, tablewriter.BgRedColor},
			{tablewriter.Bold, tablewriter.BgRedColor}, {tablewriter.Bold, tablewriter.FgBlackColor}})
		return
	}

	table.Append(genResourceRow(clusterInfo, gvk, resourceNamespace, resourceName, message))
}

func printResource(resourceStatus *libsveltosv1beta1.ResourceStatus,
	clusterNamespace, clusterName string, logger logr.Logger) error {

	clusterInfo := fmt.Sprintf("%s/%s", clusterNamespace, clusterName)
	gvk := resourceStatus.ObjectRef.GroupVersionKind().String()
	resourceNamespace := resourceStatus.ObjectRef.Namespace
	resourceName := resourceStatus.ObjectRef.Name

	if resourceStatus.Resource == nil {
		logger.V(logs.LogDebug).Info("resources are not collected. Check configuration.")
		return nil
	}

	resource, err := k8s_utils.GetUnstructured(resourceStatus.Resource)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to get resource %s:%s/%s",
			gvk, resourceNamespace, resourceName))
		return err
	}

	resourceYAML, err := yaml.Marshal(resource)
	if err != nil {
		return err
	}

	if resourceStatus.HealthStatus != libsveltosv1beta1.HealthStatusHealthy {
		red := color.New(color.FgRed).SprintfFunc()
		//nolint: forbidigo // printing results to stdout
		fmt.Println("Cluster: ", red(clusterInfo))
	} else {
		green := color.New(color.FgGreen).SprintfFunc()
		//nolint: forbidigo // printing results to stdout
		fmt.Println("Cluster: ", green(clusterInfo))
	}

	//nolint: forbidigo // printing results to stdout
	fmt.Println("Object: ", string(resourceYAML))

	return nil
}

func doConsiderResourceStatus(resourceStatus *libsveltosv1beta1.ResourceStatus,
	passedGroup, passedKind, passedNamespace string) bool {

	if passedGroup != "" {
		if !strings.EqualFold(resourceStatus.ObjectRef.GroupVersionKind().Group, passedGroup) {
			return false
		}
	}

	if passedKind != "" {
		if !strings.EqualFold(resourceStatus.ObjectRef.GroupVersionKind().Kind, passedKind) {
			return false
		}
	}

	if passedNamespace != "" {
		if resourceStatus.ObjectRef.Namespace != passedNamespace {
			return false
		}
	}

	return true
}

// Resources displays information about Kubernetes resources collected from managed clusters
func Resources(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show resources [options] [--group=<group>] [--kind=<kind>] [--namespace=<namespace>] 
  [--cluster-namespace=<name>] [--cluster=<name>] [--full] [--verbose]

     --group=<group>              Show Kubernetes resources deployed in clusters matching this group.
                                  If not specified all groups are considered.
     --kind=<kind>                Show Kubernetes resources deployed in clusters matching this Kind.
                                  If not specified all kinds are considered.
     --namespace=<namespace>      Show Kubernetes resources in this namespace. 
                                  If not specified all namespaces are considered.
     --cluster-namespace=<name>   Show Kubernetes resources in clusters in this namespace.
                                  If not specified all namespaces are considered.
     --cluster=<name>             Show Kubernetes resources in cluster with name.
                                  If not specified all cluster names are considered.
     --full                       If specified, full resources are printed

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

	full := parsedArgs["--full"].(bool)

	clusterNamespace := ""
	if passedClusterNamespace := parsedArgs["--cluster-namespace"]; passedClusterNamespace != nil {
		clusterNamespace = passedClusterNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	group := ""
	if passedGroup := parsedArgs["--group"]; passedGroup != nil {
		group = passedGroup.(string)
	}

	kind := ""
	if passedKind := parsedArgs["--kind"]; passedKind != nil {
		kind = passedKind.(string)
	}

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	return displayResources(ctx, clusterNamespace, cluster,
		group, kind, namespace, full, logger)
}
