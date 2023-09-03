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

package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	docopt "github.com/docopt/docopt-go"
	"github.com/go-logr/logr"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
)

// Show takes keyword then calls subcommand.
func Show(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show [options] <subcommand> [<args>...]

    addons        Displays information on Kubernetes addons (resources and helm releases) deployed in clusters.
    resources     Displays information from resources collected from managed clusters.
    usage         Displays information on which CAPI clusters will be affected by a policy (ClusterProfile or referenced ConfigMaps/Secrets) change.
    dryrun        Displays information on ClusterProfiles in DryRun mode. It displays what changes would
                  take effect if a ClusterProfile were to be moved out of DryRun mode.
    admin-rbac    Displays information about RBACs assigned to admins in each managed cluster.

Options:
  -h --help       Show this screen.

Description:
See 'sveltosctl show <subcommand> --help' to read about a specific subcommand.
`

	parser := &docopt.Parser{
		HelpHandler:   docopt.PrintHelpAndExit,
		OptionsFirst:  true,
		SkipHelpFlags: false,
	}

	opts, err := parser.ParseArgs(doc, nil, "1.0")
	if err != nil {
		var userError docopt.UserError
		if errors.As(err, &userError) {
			logger.V(logs.LogInfo).Info(fmt.Sprintf(
				"Invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand.\n",
				strings.Join(os.Args[1:], " "),
			))
		}
		os.Exit(1)
	}

	command := opts["<subcommand>"].(string)
	arguments := append([]string{"show", command}, opts["<args>"].([]string)...)

	if opts["<subcommand>"] != nil {
		switch command {
		case "addons":
			err = show.AddOns(ctx, arguments, logger)
		case "resources":
			err = show.Resources(ctx, arguments, logger)
		case "dryrun":
			err = show.DryRun(ctx, arguments, logger)
		case "usage":
			err = show.Usage(ctx, arguments, logger)
		case "admin-rbac":
			err = show.AdminPermissions(ctx, arguments, logger)
		default:
			//nolint: forbidigo // print doc
			fmt.Println(doc)
		}

		return err
	}
	return nil
}
