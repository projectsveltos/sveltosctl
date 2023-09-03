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

	docopt "github.com/docopt/docopt-go"
	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func updateDebuggingConfiguration(ctx context.Context, logSeverity libsveltosv1alpha1.LogLevel,
	component, namespace, clusterName, clusterType string) error {

	cc, err := collectLogLevelConfiguration(ctx, namespace, clusterName, clusterType)
	if err != nil {
		return nil
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
			break
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

	return updateLogLevelConfiguration(ctx, spec, namespace, clusterName, clusterType)
}

// Set displays/changes log verbosity for a given component
func Set(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level set --component=<name> [--namespace=<namespace>] [--cluster=<cluster-name>] [--cluster-type=<cluster-type>] (--info|--debug|--verbose)
Options:
  -h --help                		   Show this screen.
     --component=<name>    		   Name of the component for which log severity is being set.
     --namespace=<namespace> 	   Namespace of the cluster.
     --cluster=<cluster-name> 	   Name of the cluster.
     --cluster-type=<cluster-type> Type of the cluster (Capi or Sveltos).
     --info                		   Set log severity to info.
     --debug               		   Set log severity to debug.
     --verbose             		   Set log severity to verbose.
	 
Description:
  The log-level set command sets log severity for the specified component in the specified cluster.
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
	if passedClusterName := parsedArgs["--cluster"]; passedClusterName != nil {
        clusterName = passedClusterName.(string)
    }
	clusterType := ""
	if passedClusterType := parsedArgs["--cluster-type"]; passedClusterType != nil {
        clusterType = passedClusterType.(string)
    }

	info := parsedArgs["--info"].(bool)
	debug := parsedArgs["--debug"].(bool)
	verbose := parsedArgs["--verbose"].(bool)

	var logSeverity libsveltosv1alpha1.LogLevel
	if info {
		logSeverity = libsveltosv1beta1.LogLevelInfo
	} else if debug {
		logSeverity = libsveltosv1beta1.LogLevelDebug
	} else if verbose {
		logSeverity = libsveltosv1beta1.LogLevelVerbose
	}

	return updateDebuggingConfiguration(ctx, logSeverity, component, namespace, clusterName, clusterType)
}
