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
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func updateDebuggingConfiguration(ctx context.Context, logSeverity libsveltosv1beta1.LogLevel,
	component string) error {

	cc, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return nil // Question: this should return 'err' instead of nil?
	}

	found := false
	spec := make([]libsveltosv1beta1.ComponentConfiguration, len(cc))

	for i, c := range cc {
		if string(c.component) == component {
			spec[i] = libsveltosv1beta1.ComponentConfiguration{
				Component: c.component,
				LogLevel:  logSeverity,
			}
			found = true
		} else {
			spec[i] = libsveltosv1beta1.ComponentConfiguration{
				Component: c.component,
				LogLevel:  c.logSeverity,
			}
		}
	}

	if !found {
		spec = append(spec,
			libsveltosv1beta1.ComponentConfiguration{
				Component: libsveltosv1beta1.Component(component),
				LogLevel:  logSeverity,
			},
		)
	}

	return updateLogLevelConfiguration(ctx, spec)
}

func updateDebuggingConfigurationInManaged(ctx context.Context, logSeverity libsveltosv1beta1.LogLevel,
	component, namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType) error {

	// Get client for the managed cluster using libsveltos the clusterproxy
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

	// Get existing configuration from the managed cluster
	cc, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return err
	}

	found := false
	spec := make([]libsveltosv1beta1.ComponentConfiguration, len(cc))

	for i, c := range cc {
		if string(c.component) == component {
			spec[i] = libsveltosv1beta1.ComponentConfiguration{
				Component: c.component,
				LogLevel:  logSeverity,
			}
			found = true
		} else {
			spec[i] = libsveltosv1beta1.ComponentConfiguration{
				Component: c.component,
				LogLevel:  c.logSeverity,
			}
		}
	}

	if !found {
		spec = append(spec,
			libsveltosv1beta1.ComponentConfiguration{
				Component: libsveltosv1beta1.Component(component),
				LogLevel:  logSeverity,
			},
		)
	}

	return updateLogLevelConfigurationWithClient(ctx, managedClient, spec)
}

// Set displays/changes log verbosity for a given component
func Set(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level set --component=<name> (--info|--debug|--verbose) [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                    Show this screen.
     --component=<name>        Name of the component for which log severity is being set.
     --info                    Set log severity to info.
     --debug                   Set log severity to debug.
     --verbose                 Set log severity to verbose.
     --namespace=<namespace>   (Optional) Namespace where the managed cluster is located.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of cluster: Capi or Sveltos.
	 
Description:
  The log-level set command set log severity for the specified component.
  If namespace and clusterName are provided, the log level is set in the managed cluster.
  Otherwise, it is set in the management cluster.
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

	component := ""
	if passedComponent := parsedArgs["--component"]; passedComponent != nil {
		component = passedComponent.(string)
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

	info := parsedArgs["--info"].(bool)
	debug := parsedArgs["--debug"].(bool)
	verbose := parsedArgs["--verbose"].(bool)

	var logSeverity libsveltosv1beta1.LogLevel
	if info {
		logSeverity = libsveltosv1beta1.LogLevelInfo
	} else if debug {
		logSeverity = libsveltosv1beta1.LogLevelDebug
	} else if verbose {
		logSeverity = libsveltosv1beta1.LogLevelVerbose
	}

	if namespace != "" && clusterName != "" {
		return updateDebuggingConfigurationInManaged(ctx, logSeverity, component, namespace, clusterName, clusterType)
	}
	return updateDebuggingConfiguration(ctx, logSeverity, component)
}
