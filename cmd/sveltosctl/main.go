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

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	docopt "github.com/docopt/docopt-go"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/commands"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

type clusterAccess struct {
	scheme     *runtime.Scheme
	restConfig *rest.Config
	clientSet  *kubernetes.Clientset
	client     client.Client
}

func main() {
	doc := `Usage:
	sveltosctl [options] <command> [<args>...]

    show           Display information on deployed Kubernetes addons (resources and helm releases) in each cluster
                   or for ClusterProfiles in DryRun mode, what changes would take effect if the ClusterProfile
                   mode was to be moved out of DryRun mode. Displays also information on which permissions each
                   tenant admin has in each managed cluster.
    register       Onboard an existing non CAPI cluster by creating all necessary internal resources.
    deregister     Remove a non CAPI cluster that was previously registered with Sveltos.
    generate       Generates a Kubeconfig that can later be used to register a cluster.
                   Run this command with sveltosctl pointing to the cluster you want Sveltos to manage.
    log-level      Allows changing the log verbosity.
    version        Display the version of sveltosctl.

Options:
	-h --help          Show this screen.

Description:
  The sveltosctl command line tool is used to display various type of information
  regarding policies deployed in each cluster.
  See 'sveltosctl <command> --help' to read about a specific subcommand.

  To reach cluster:
  - KUBECONFIG environment variable pointing at a file
  - In-cluster config if running in cluster
  - $HOME/.kube/config if exists
`
	klog.InitFlags(nil)

	ctx := context.Background()

	ctrl.SetLogger(klog.Background())
	logger := klog.FromContext(ctx)

	access, err := initializeManagementClusterAccess()
	if err != nil {
		_ = commands.Version(nil, logger)
		return
	}

	utils.InitalizeManagementClusterAcces(access.scheme, access.restConfig,
		access.clientSet, access.client)

	parser := &docopt.Parser{
		HelpHandler:   docopt.PrintHelpOnly,
		OptionsFirst:  true,
		SkipHelpFlags: false,
	}

	opts, err := parser.ParseArgs(doc, nil, "")
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

	if opts["<command>"] != nil {
		command := opts["<command>"].(string)
		args := append([]string{command}, opts["<args>"].([]string)...)
		var err error

		switch command {
		case "show":
			err = commands.Show(ctx, args, logger)
		case "register":
			err = commands.RegisterCluster(ctx, args, logger)
		case "deregister":
			err = commands.DeregisterCluster(ctx, args, logger)
		case "generate":
			err = commands.Generate(ctx, args, logger)
		case "log-level":
			err = commands.LogLevel(ctx, args, logger)
		case "version":
			err = commands.Version(args, logger)
		default:
			err = fmt.Errorf("unknown command: %q\n%s", command, doc)
		}

		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("%v\n", err))
		}
	}
}

func initializeManagementClusterAccess() (*clusterAccess, error) {
	scheme, err := utils.GetScheme()
	if err != nil {
		werr := fmt.Errorf("failed to get scheme %w", err)
		return nil, werr
	}

	restConfig, err := ctrl.GetConfig()
	if err != nil {
		werr := fmt.Errorf("failed to get config %w", err)
		return nil, werr
	}
	restConfig.QPS = 100
	restConfig.Burst = 100

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		werr := fmt.Errorf("error in getting access to K8S: %w", err)
		return nil, werr
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		werr := fmt.Errorf("failed to connect: %w", err)
		return nil, werr
	}

	access := clusterAccess{
		scheme:     scheme,
		restConfig: restConfig,
		clientSet:  cs,
		client:     c,
	}

	return &access, nil
}
