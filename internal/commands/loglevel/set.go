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
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func updateDebuggingConfiguration(ctx context.Context, logSeverity libsveltosv1alpha1.LogLevel, component, namespace string, clusterName string) error {
    cc, err := collectLogLevelConfiguration(ctx, namespace, clusterName)
    if err != nil {
        return err
    }

    found := false
    spec := make([]libsveltosv1alpha1.ComponentConfiguration, len(cc))

    for i, c := range cc {
        if string(c.component) == component {
            spec[i] = libsveltosv1alpha1.ComponentConfiguration{
                Component: c.component,
                LogLevel:  logSeverity,
            }
            found = true
            break
        } else {
            spec[i] = libsveltosv1alpha1.ComponentConfiguration{
                Component: c.component,
                LogLevel:  c.logSeverity,
            }
        }
    }

    if !found {
        spec = append(spec, libsveltosv1alpha1.ComponentConfiguration{
            Component: libsveltosv1alpha1.Component(component),
            LogLevel:  logSeverity,
        })
    }

    return updateLogLevelConfiguration(ctx, namespace, clusterName, spec)
}

// set changes log verbosity for a given component
func Set(ctx context.Context, args []string) error {
    doc := `Usage:
  sveltosctl log-level set --component=<name> [--cluster-namespace=<namespace>] [--cluster-name=<name>] (--info|--debug|--verbose)
Options:
  -h --help                     Show this screen.
     --component=<name>         Name of the component for which log severity is being set.
     --cluster-namespace=<namespace> Optional cluster namespace.
     --cluster-name=<name>      Optional cluster name.
     --info                     Set log severity to info.
     --debug                    Set log severity to debug.
     --verbose                  Set log severity to verbose.
	 
Description:
  The log-level set command sets log severity for the specified component, optionally in a specified managed cluster.
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

	component := ""
	if passedComponent := parsedArgs["--component"]; passedComponent != nil {
		component = passedComponent.(string)
	}
    namespace := parsedArgs["--cluster-namespace"].(string) // may be nil
    clusterName := parsedArgs["--cluster-name"].(string)    // may be nil

    var logSeverity libsveltosv1alpha1.LogLevel
    if parsedArgs["--info"].(bool) {
        logSeverity = libsveltosv1alpha1.LogLevelInfo
    } else if parsedArgs["--debug"].(bool) {
        logSeverity = libsveltosv1alpha1.LogLevelDebug
    } else if parsedArgs["--verbose"].(bool) {
        logSeverity = libsveltosv1alpha1.LogLevelVerbose
    }

    return updateDebuggingConfiguration(ctx, logSeverity, component, namespace, clusterName)
}
