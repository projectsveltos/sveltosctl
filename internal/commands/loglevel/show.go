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
	"github.com/olekukonko/tablewriter"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

func showLogSettings(ctx context.Context) error {
	componentConfiguration, err := collectLogLevelConfiguration(ctx)
	if err != nil {
		return err
	}

	return renderLogSettings(componentConfiguration)
}

func showLogSettingsInManaged(ctx context.Context, namespace, clusterName string,
	clusterType libsveltosv1beta1.ClusterType) error {

	managedClient, err := getManagedClusterClient(ctx, namespace, clusterName, clusterType)
	if err != nil {
		return err
	}

	componentConfiguration, err := collectLogLevelConfigurationFromClient(ctx, managedClient)
	if err != nil {
		return err
	}

	return renderLogSettings(componentConfiguration)
}

func renderLogSettings(cc []*componentConfiguration) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"COMPONENT", "VERBOSITY"})

	for _, c := range cc {
		if err := table.Append([]string{string(c.component), string(c.logSeverity)}); err != nil {
			return err
		}
	}

	return table.Render()
}

// Show displays information about log verbosity (if set).
//
// When --namespace and --clusterName are provided, settings are read from the
// specified managed cluster. Otherwise they are read from the management
// cluster.
func Show(ctx context.Context, args []string) error {
	doc := `Usage:
  sveltosctl log-level show [--namespace=<namespace>] [--clusterName=<cluster-name>] [--clusterType=<cluster-type>]
Options:
  -h --help                        Show this screen.
     --namespace=<namespace>       (Optional) Namespace of the managed cluster.
     --clusterName=<cluster-name>  (Optional) Name of the managed cluster.
     --clusterType=<cluster-type>  (Optional) Type of managed cluster: Capi or Sveltos. Defaults to Capi.

Description:
  The log-level show command shows information about current log verbosity.
  If --namespace and --clusterName are provided, settings are read from the
  specified managed cluster. Otherwise they are read from the management
  cluster.
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

	namespace, clusterName, clusterType, err := parseManagedClusterArgs(parsedArgs)
	if err != nil {
		return err
	}

	if namespace != "" && clusterName != "" {
		return showLogSettingsInManaged(ctx, namespace, clusterName, clusterType)
	}
	return showLogSettings(ctx)
}
