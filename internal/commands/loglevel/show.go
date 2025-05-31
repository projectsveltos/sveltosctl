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

package loglevel

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func showLogSettings(ctx context.Context) error {
	componentConfiguration, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"COMPONENT", "VERBOSITY"})
	genRow := func(component, verbosity string) []string {
		return []string{
			component,
			verbosity,
		}
	}

	for _, c := range componentConfiguration {
		table.Append(genRow(string(c.component), string(c.logSeverity)))
	}

	table.Render()
	return nil
}

func showLogSettingsInManaged(ctx context.Context, namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType) error {
	// Get client for the managed cluster using libsveltos clusterproxy
	cluster := &corev1.ObjectReference{
		Namespace: namespace,
		Name:      clusterName,
	}
	
	// Set the appropriate Kind and APIVersion based on the clusterType
	if clusterType == libsveltosv1beta1.ClusterTypeCapi {
		cluster.Kind = "Cluster"
		cluster.APIVersion = "cluster.x-k8s.io/v1beta1"
	} else {
		cluster.Kind = "SveltosCluster"
		cluster.APIVersion = "lib.projectsveltos.io/v1beta1"
	}

	logger := logr.Discard() // Use a discard logger for simplicity
	managedClient, err := clusterproxy.GetKubernetesClient(ctx, utils.GetAccessInstance().GetClient(),
		cluster.Namespace, cluster.Name, "", "", clusterType, logger)
	if err != nil {
		return fmt.Errorf("failed to get client for managed cluster %s/%s: %w", namespace, clusterName, err)
	}

	// Get configuration from the managed cluster
	componentConfiguration, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"COMPONENT", "VERBOSITY"})
	genRow := func(component, verbosity string) []string {
		return []string{
			component,
			verbosity,
		}
	}

	for _, c := range componentConfiguration {
		table.Append(genRow(string(c.component), string(c.logSeverity)))
	}

	table.Render()
	return nil
}

// Show displays information about log verbosity (if set)
func Show(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level show [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                    Show this screen.
     --namespace=<namespace>   (Optional) Namespace where the managed cluster is located.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of cluster: Capi or Sveltos.
     
Description:
  The log-level show command shows information about current log verbosity.
  If namespace and clusterName are provided, the log levels are shown from the managed cluster.
  Otherwise, they are shown from the management cluster.
`
	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand",
			strings.Join(args, " "),
		)
	}
	if len(parsedArgs) == 0 {
		return nil
	}

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	clusterName := ""
	if passedClusterName := parsedArgs["--clusterName"]; passedClusterName != nil {
		clusterName = passedClusterName.(string)
	}

	clusterType := libsveltosv1beta1.ClusterTypeCapi // default
	if passedClusterType := parsedArgs["--clusterType"]; passedClusterType != nil {
		clusterTypeStr := passedClusterType.(string)
		if clusterTypeStr == "Sveltos" {
			clusterType = libsveltosv1beta1.ClusterTypeSveltos
		}
	}

	if namespace != "" && clusterName != "" {
		return showLogSettingsInManaged(ctx, namespace, clusterName, clusterType)
	}
	return showLogSettings(ctx)
}
