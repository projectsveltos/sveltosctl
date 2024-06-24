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

package snapshot

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
	// snapshotName is the name of the Snaphost instance which caused a snapshot to be collected
	// snapshotDate is a string containing the Date a snapshot was taken
	genListSnapshotRow = func(snaphostName, snapshotDate string,
	) []string {
		return []string{
			snaphostName,
			snapshotDate,
		}
	}
)

func doConsiderSnapshot(snaphostInstance *utilsv1beta1.Snapshot, passedSnapshot string) bool {
	if passedSnapshot == "" {
		return true
	}

	return snaphostInstance.Name == passedSnapshot
}

func listSnapshots(ctx context.Context, passedSnapshotName string, logger logr.Logger) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"SNAPSHOT POLICY", "DATE"})

	if err := displaySnapshots(ctx, passedSnapshotName, table, logger); err != nil {
		return err
	}

	table.Render()

	return nil
}

func displaySnapshots(ctx context.Context, passedSnapshotName string,
	table *tablewriter.Table, logger logr.Logger) error {

	snapshotList := &utilsv1beta1.SnapshotList{}
	logger.V(logs.LogDebug).Info("List all Snapshot instances")
	instance := utils.GetAccessInstance()
	err := instance.ListResources(ctx, snapshotList)
	if err != nil {
		return err
	}
	for i := range snapshotList.Items {
		if doConsiderSnapshot(&snapshotList.Items[i], passedSnapshotName) {
			err = displaySnapshot(&snapshotList.Items[i], table, logger)
			if err != nil {
				return nil
			}
		}
	}

	return nil
}

func displaySnapshot(snapshotInstance *utilsv1beta1.Snapshot,
	table *tablewriter.Table, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering Snapshot instance %s", snapshotInstance.Name))
	snapshotClient := collector.GetClient()
	results, err := snapshotClient.ListCollections(snapshotInstance.Spec.Storage, snapshotInstance.Name,
		collector.Snapshot, logger)
	if err != nil {
		return err
	}
	for i := range results {
		table.Append(genListSnapshotRow(snapshotInstance.Name, results[i]))
	}
	return nil
}

// List collects snapshot
func List(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
	sveltosctl snapshot list [options] [--snapshot=<name>] [--verbose]

     --snapshot=<name>      List snapshots taken because of Snapshot instance with that name

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The snapshot list command lists all snapshots taken.
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
	if passedSnapshotName := parsedArgs["--snapshot"]; passedSnapshotName != nil {
		snapshostName = passedSnapshotName.(string)
	}

	return listSnapshots(ctx, snapshostName, logger)
}
