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

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

func unsetDebuggingConfiguration(ctx context.Context, component string) error {
	cc, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return nil
	}

	spec, found := removeComponent(cc, component)
	if !found {
		return nil
	}
	return updateLogLevelConfiguration(ctx, spec)
}

func unsetDebuggingConfigurationInManaged(ctx context.Context, component, namespace,
	clusterName string, clusterType libsveltosv1beta1.ClusterType) error {

	managedClient, err := getManagedClusterClient(ctx, namespace, clusterName, clusterType)
	if err != nil {
		return err
	}

	cc, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return err
	}

	spec, found := removeComponent(cc, component)
	if !found {
		return nil
	}
	return updateLogLevelConfigurationWithClient(ctx, managedClient, spec)
}

// removeComponent returns the ComponentConfiguration slice with component
// removed, along with a flag indicating whether it was present.
func removeComponent(cc []*componentConfiguration,
	component string) ([]libsveltosv1beta1.ComponentConfiguration, bool) {

	spec := make([]libsveltosv1beta1.ComponentConfiguration, 0, len(cc))
	found := false
	for _, c := range cc {
		if string(c.component) == component {
			found = true
			continue
		}
		spec = append(spec, libsveltosv1beta1.ComponentConfiguration{
			Component: c.component,
			LogLevel:  c.logSeverity,
		})
	}
	return spec, found
}

// Unset resets log verbosity for a given component.
//
// When --namespace and --clusterName are provided, the DebuggingConfiguration
// is updated in the specified managed cluster. Otherwise, it is updated in the
// management cluster.
func Unset(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level unset --component=<name> [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                        Show this screen.
     --component=<name>            Name of the component for which log severity is being unset.
     --namespace=<namespace>       (Optional) Namespace of the managed cluster.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of managed cluster: Capi or Sveltos. Defaults to Capi.

Description:
  The log-level unset command unsets log severity for the specified component.
  If --namespace and --clusterName are provided, log severity is unset in the
  specified managed cluster. Otherwise it is unset in the management cluster.
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

	namespace, clusterName, clusterType, err := parseManagedClusterArgs(parsedArgs)
	if err != nil {
		return err
	}

	if namespace != "" && clusterName != "" {
		return unsetDebuggingConfigurationInManaged(ctx, component, namespace, clusterName, clusterType)
	}
	return unsetDebuggingConfiguration(ctx, component)
}
