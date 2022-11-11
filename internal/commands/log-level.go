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
	"github.com/projectsveltos/sveltosctl/internal/commands/loglevel"
)

// LogLevel allows changing log verbosity.
func LogLevel(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
	sveltosctl log-level <command> [<args>...]

	show          Show current log severity configuration.
	set           Set log severity.
	unset         Remove log severity setting for a given component.

Options:
	-h --help      Show this screen.

Description:
	See 'sveltosctl log-level <command> --help' to read about a specific subcommand.
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

	command := opts["<command>"].(string)
	arguments := append([]string{"logLevel", command}, opts["<args>"].([]string)...)

	switch command {
	case "show":
		return loglevel.Show(ctx, arguments)
	case "set":
		return loglevel.Set(ctx, arguments)
	case "unset":
		return loglevel.Unset(ctx, arguments)
	default:
		//nolint: forbidigo // print doc
		fmt.Println(doc)
	}

	return nil
}
