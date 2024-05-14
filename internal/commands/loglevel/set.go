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

func updateDebuggingConfiguration(ctx context.Context, logSeverity libsveltosv1alpha1.LogLevel,
    component string, dc *libsveltosv1alpha1.DebuggingConfiguration,) error {

    cc, err := collectLogLevelConfiguration(ctx, dc)
    if err != nil {
        return nil
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
        spec = append(spec,
            libsveltosv1alpha1.ComponentConfiguration{
                Component: libsveltosv1alpha1.Component(component),
                LogLevel:  logSeverity,
            },
        )
    }

    return updateLogLevelConfiguration(ctx, spec, dc)
}

// set changes log verbosity for a given component
func Set(ctx context.Context, args []string) error {
    doc := `Usage:
  sveltosctl log-level set --component=<name> (--info|--debug|--verbose) [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                  	  Show this screen.
     --component=<name>      	  Name of the component for which log severity is being set.
     --info                  	  Set log severity to info.
     --debug                 	  Set log severity to debug.
     --verbose               	  Set log severity to verbose.
     --namespace=<namespace> 	  Namespace in the managed cluster (optional).
     --clusterName=<cluster-name> Name of the managed cluster (optional).
     --clusterType=<cluster-type> Type of the managed cluster (optional).
	 
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

    info := parsedArgs["--info"].(bool)
    debug := parsedArgs["--debug"].(bool)
    verbose := parsedArgs["--verbose"].(bool)

    namespace := ""
    if parsedNamespace := parsedArgs["--namespace"]; parsedNamespace != nil {
        namespace = parsedNamespace.(string)
    }

    clusterName := ""
    if parsedClusterName := parsedArgs["--clusterName"]; parsedClusterName != nil {
        clusterName = parsedClusterName.(string)
    }

    clusterType := ""
    if parsedClusterType := parsedArgs["--clusterType"]; parsedClusterType != nil {
        clusterType = parsedClusterType.(string)
    }

    var logSeverity libsveltosv1alpha1.LogLevel
    if info {
        logSeverity = libsveltosv1alpha1.LogLevelInfo
    } else if debug {
        logSeverity = libsveltosv1alpha1.LogLevelDebug
    } else if verbose {
        logSeverity = libsveltosv1alpha1.LogLevelVerbose
    }

    // if namespace, clusterName, and clusterType are provided, update the configuration in the managed cluster
    if namespace != "" && clusterName != "" && clusterType != "" {
        return updateDebuggingConfigurationInManaged(ctx, logSeverity, component, namespace, clusterName, clusterType)
    }

	instance := utils.GetAccessInstance()
	dc, err := instance.GetDebuggingConfiguration(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("DebuggingConfiguration not found")
		}
		return err
	}
	
	cc, err := collectLogLevelConfiguration(ctx, dc)  // pass dc
	if err != nil {
		return err
	}

    return updateLogLevelConfiguration(ctx, spec, dc)  // Pass dc
}
