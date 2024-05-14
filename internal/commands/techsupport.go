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
	"github.com/projectsveltos/sveltosctl/internal/commands/techsupport"
)

// Techsupport takes keyword then calls subcommand.
func Techsupport(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl techsupport [options] <subcommand> [<args>...]

    list          Displays all available collected techsupports.

Options:
  -h --help       Show this screen.

Description:
See 'sveltosctl techsupport <subcommand> --help' to read about a specific subcommand.
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
	arguments := append([]string{"techsupport", command}, opts["<args>"].([]string)...)

	if opts["<subcommand>"] != nil {
		switch command {
		case "list":
			err = techsupport.List(ctx, arguments, logger)
		default:
			//nolint: forbidigo // print doc
			fmt.Println(doc)
		}

		return err
	}
	return nil
}
