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

	docopt "github.com/docopt/docopt-go"
	"github.com/olekukonko/tablewriter"
)

func showLogSettings(ctx context.Context, namespace, clusterName, clusterType string) error {
	componentConfiguration, err := collectLogLevelConfiguration(ctx, namespace, clusterName, clusterType)
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
  sveltosctl log-level show [--namespace=<namespace>] [--cluster=<cluster-name>] [--cluster-type=<cluster-type>]
Options:
  	-h --help                        Show this screen.
       --namespace=<namespace>       Namespace of the cluster.
       --cluster=<cluster-name>      Name of the cluster.
       --cluster-type=<cluster-type> Type of the cluster (Capi or Sveltos).
     
Description:
  The log-level show command shows information about current log verbosity in the specified cluster.
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

	namespace := parsedArgs["--namespace"].(string)
	clusterName := parsedArgs["--cluster"].(string)
	clusterType := parsedArgs["--cluster-type"].(string)

	return showLogSettings(ctx, namespace, clusterName, clusterType)
}