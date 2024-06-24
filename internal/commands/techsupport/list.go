/*
Copyright 2023. projectsveltos.io. All rights reserved.

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

package techsupport

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"

	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
)

var (
	// techsupportName is the name of the techsupport instance which caused a techsupport to be collected
	// techsupportDate is a string containing the Date a techsupport was taken
	genListTechsupportRow = func(techsupportName, techsupportDate string,
	) []string {
		return []string{
			techsupportName,
			techsupportDate,
		}
	}
)

func doConsiderTechsupport(techsupportInstance *utilsv1beta1.Techsupport, passedTechsupport string) bool {
	if passedTechsupport == "" {
		return true
	}

	return techsupportInstance.Name == passedTechsupport
}

func listTechsupports(ctx context.Context, passedTechsupportName string, logger logr.Logger) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"TECHSUPPORT POLICY", "DATE"})

	if err := displayTechsupports(ctx, passedTechsupportName, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displayTechsupports(ctx context.Context, passedTechsupportName string,
	table *tablewriter.Table, logger logr.Logger) error {

	techsupportList := &utilsv1beta1.TechsupportList{}
	logger.V(logs.LogDebug).Info("List all Techsupport instances")
	instance := utils.GetAccessInstance()
	err := instance.ListResources(ctx, techsupportList)
	if err != nil {
		return err
	}
	for i := range techsupportList.Items {
		if doConsiderTechsupport(&techsupportList.Items[i], passedTechsupportName) {
			err = displayTechsupport(&techsupportList.Items[i], table, logger)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func displayTechsupport(techsupportInstance *utilsv1beta1.Techsupport,
	table *tablewriter.Table, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering Techsupport instance %s", techsupportInstance.Name))
	techsupportClient := collector.GetClient()
	results, err := techsupportClient.ListCollections(techsupportInstance.Spec.Storage, techsupportInstance.Name,
		collector.Techsupport, logger)
	if err != nil {
		return err
	}
	for i := range results {
		table.Append(genListTechsupportRow(techsupportInstance.Name, results[i]))
	}
	return nil
}

// List collects techsupport
func List(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
	sveltosctl techsupport list [options] [--techsupport=<name>] [--verbose]

     --techsupport=<name>      List techsupports taken because of Techsupport instance with that name

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The techsupport list command lists all techsupports taken.
`

	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug))
		if err != nil {
			return err
		}
	}

	snapshostName := ""
	if passedTechsupportName := parsedArgs["--techsupport"]; passedTechsupportName != nil {
		snapshostName = passedTechsupportName.(string)
	}

	return listTechsupports(ctx, snapshostName, logger)
}
