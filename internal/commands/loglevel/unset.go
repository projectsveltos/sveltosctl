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

func unsetDebuggingConfiguration(ctx context.Context, component string) error {
	cc, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return nil
	}

	found := false
	spec := make([]libsveltosv1beta1.ComponentConfiguration, 0)

	for _, c := range cc {
		if string(c.component) == component {
			found = true
			continue
		} else {
			spec = append(spec,
				libsveltosv1beta1.ComponentConfiguration{
					Component: c.component,
					LogLevel:  c.logSeverity,
				},
			)
		}
	}

	if found {
		return updateLogLevelConfiguration(ctx, spec)
	}
	return nil
}

func unsetDebuggingConfigurationInManaged(ctx context.Context, component, namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType) error {
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

	// Get existing configuration from the managed cluster
	cc, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return nil
	}

	found := false
	spec := make([]libsveltosv1beta1.ComponentConfiguration, 0)

	for _, c := range cc {
		if string(c.component) == component {
			found = true
			continue
		} else {
			spec = append(spec,
				libsveltosv1beta1.ComponentConfiguration{
					Component: c.component,
					LogLevel:  c.logSeverity,
				},
			)
		}
	}

	if found {
		return updateLogLevelConfigurationWithClient(ctx, managedClient, spec)
	}
	return nil
}

// Unset resets log verbosity for a given component
func Unset(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level unset --component=<name> [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                    Show this screen.
     --component=<name>        Name of the component for which log severity is being unset.
     --namespace=<namespace>   (Optional) Namespace where the managed cluster is located.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of cluster: Capi or Sveltos.
	 
Description:
  The log-level unset command unsets log severity for the specified component.
  If namespace and clusterName are provided, the log level is unset in the managed cluster.
  Otherwise, it is unset in the management cluster.
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

	if namespace != "" && clusterName != "" {
		return unsetDebuggingConfigurationInManaged(ctx, component, namespace, clusterName, clusterType)
	}
	return unsetDebuggingConfiguration(ctx, component)
}
