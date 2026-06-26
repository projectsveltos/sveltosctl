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

func updateDebuggingConfiguration(ctx context.Context, logSeverity libsveltosv1beta1.LogLevel,
	component string) error {

	cc, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return nil
	}

	spec := buildUpdatedSpec(cc, component, logSeverity)
	return updateLogLevelConfiguration(ctx, spec)
}

func updateDebuggingConfigurationInManaged(ctx context.Context, logSeverity libsveltosv1beta1.LogLevel,
	component, namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType) error {

	managedClient, err := getManagedClusterClient(ctx, namespace, clusterName, clusterType)
	if err != nil {
		return err
	}

	cc, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return err
	}

	spec := buildUpdatedSpec(cc, component, logSeverity)
	return updateLogLevelConfigurationWithClient(ctx, managedClient, spec)
}

// buildUpdatedSpec returns the ComponentConfiguration slice that would result
// from setting component to logSeverity, preserving existing entries for other
// components.
func buildUpdatedSpec(cc []*componentConfiguration, component string,
	logSeverity libsveltosv1beta1.LogLevel) []libsveltosv1beta1.ComponentConfiguration {

	spec := make([]libsveltosv1beta1.ComponentConfiguration, len(cc))
	found := false
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

	return spec
}

// Set displays/changes log verbosity for a given component.
//
// When --namespace and --clusterName are provided, the DebuggingConfiguration
// is updated in the specified managed cluster. Otherwise, it is updated in the
// management cluster.
func Set(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level set --component=<name> (--info|--debug|--verbose) [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                        Show this screen.
     --component=<name>            Name of the component for which log severity is being set.
     --info                        Set log severity to info.
     --debug                       Set log severity to debug.
     --verbose                     Set log severity to verbose.
     --namespace=<namespace>       (Optional) Namespace of the managed cluster.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of managed cluster: Capi or Sveltos. Defaults to Capi.

Description:
  The log-level set command set log severity for the specified component.
  If --namespace and --clusterName are provided, log severity is set in the
  specified managed cluster. Otherwise it is set in the management cluster.
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

	info := parsedArgs["--info"].(bool)
	debug := parsedArgs["--debug"].(bool)
	verbose := parsedArgs["--verbose"].(bool)

	var logSeverity libsveltosv1beta1.LogLevel
	switch {
	case info:
		logSeverity = libsveltosv1beta1.LogLevelInfo
	case debug:
		logSeverity = libsveltosv1beta1.LogLevelDebug
	case verbose:
		logSeverity = libsveltosv1beta1.LogLevelVerbose
	}

	if namespace != "" && clusterName != "" {
		return updateDebuggingConfigurationInManaged(ctx, logSeverity, component, namespace, clusterName, clusterType)
	}
	return updateDebuggingConfiguration(ctx, logSeverity, component)
}
