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
)

func unsetDebuggingConfiguration(ctx context.Context, component, namespace, clusterName string) error {
    cc, err := collectLogLevelConfiguration(ctx, namespace, clusterName)
    if err != nil {
        return err
    }

    found := false
    spec := make([]libsveltosv1alpha1.ComponentConfiguration, 0)

    for _, c := range cc {
        if string(c.component) == component {
            found = true
            continue
        } else {
            spec = append(spec, libsveltosv1alpha1.ComponentConfiguration{
                Component: c.component,
                LogLevel:  c.logSeverity,
            })
        }
    }

    if found {
        return updateLogLevelConfiguration(ctx, spec, namespace, clusterName)
    }
    return nil
}



// Unset resets log verbosity for a given component
func Unset(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level unset --component=<name> [--namespace=<namespace>] [--cluster=<cluster>]
Options:
  -h --help               Show this screen.
     --component=<name>   Name of the component for which log severity is being set.
     --namespace=<namespace>   Namespace of the component (default "default").
     --cluster=<cluster>       Cluster name (optional).
	 
Description:
  The log-level unset command removes log severity settings for the specified component.
`
	parsedArgs, err := docopt.ParseArgs(doc, args, "1.0")
	if err != nil {
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand",
			strings.Join(args, " "),
		)
	}
	if len(parsedArgs) == 0 {
		return nil
	}

	component := parsedArgs["--component"].(string)
	namespace := "default"
	if ns, ok := parsedArgs["--namespace"]; ok && ns != nil {
		namespace = ns.(string)
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

	clusterName := ""
	if cl, ok := parsedArgs["--cluster"]; ok && cl != nil {
		clusterName = cl.(string)
	}

	return unsetDebuggingConfiguration(ctx, component, namespace, clusterName)
}

