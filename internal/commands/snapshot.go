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
	"reflect"
	"strings"

	docopt "github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/commands/snapshot"
	"github.com/projectsveltos/sveltosctl/internal/logs"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// Snapshot takes keyword then calls subcommand.
func Snapshot(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl snapshot [options] <subcommand> [<args>...]

    list          Displays all available collected snapshots.
    diff          Displays diff between two collected snapshots.
    rollback      Rollback to any previous configuration snapshot.
    reconciler    Starts a snapshot reconciler.

Options:
  -h --help       Show this screen.

Description:
See 'sveltosctl snapshot <subcommand> --help' to read about a specific subcommand.
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
	arguments := append([]string{"snapshot", command}, opts["<args>"].([]string)...)

	if opts["<subcommand>"] == "reconciler" {
		if err = watchResources(ctx, logger); err != nil {
			logger.Error(err, "failed to watch resource")
			return err
		}
		select {}
	} else if opts["<subcommand>"] != nil {
		switch command {
		case "list":
			err = snapshot.List(ctx, arguments, logger)
		case "diff":
			err = snapshot.Diff(ctx, arguments, logger)
		case "rollback":
			err = snapshot.Rollback(ctx, arguments, logger)
		default:
			//nolint: forbidigo // print doc
			fmt.Println(doc)
		}

		return err
	}
	return nil
}

func watchResources(ctx context.Context, logger logr.Logger) error {
	scheme, _ := utils.GetScheme()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		logger.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Create an un-managed controller
	c, err := controller.NewUnmanaged("sveltoswacther", mgr, controller.Options{
		Reconciler:              reconcile.Func(SnapshotReconciler),
		MaxConcurrentReconciles: 1,
	})

	if err != nil {
		logger.Error(err, "unable to create watcher")
		return err
	}

	if err := c.Watch(&source.Kind{Type: &utilsv1alpha1.Snapshot{}},
		handler.EnqueueRequestsFromMapFunc(snapshotHandler),
		addModifyDeletePredicates(),
	); err != nil {
		logger.Error(err, "unable to watch resource Snapshot")
		return err
	}

	// Start controller in a goroutine so not to block.
	go func() {
		const workerNumber = 10
		snapshotter.InitializeClient(ctx, logger.WithName("snapshotter"), mgr.GetClient(), workerNumber)
		// Start controller. This will block until the context is
		// closed, or the controller returns an error.
		logger.Info("Starting watcher controller")
		if err := c.Start(ctx); err != nil {
			logger.Error(err, "cannot run controller")
			panic(1)
		}
	}()

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "unable to continue running manager")
		return err
	}

	return nil
}

func addModifyDeletePredicates() predicate.Funcs {
	logger := klogr.New()
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			newObject := e.ObjectNew.(*utilsv1alpha1.Snapshot)
			oldObject := e.ObjectOld.(*utilsv1alpha1.Snapshot)
			logger.Info(fmt.Sprintf("Update kind: %s Info: %s/%s",
				newObject.GetObjectKind().GroupVersionKind().Kind,
				newObject.GetNamespace(), newObject.GetName()))

			if oldObject == nil ||
				!reflect.DeepEqual(newObject.Spec, oldObject.Spec) {

				return true
			}

			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			object := e.Object
			o := e.Object
			logger.Info(fmt.Sprintf("Delete kind: %s Info: %s/%s",
				object.GetObjectKind().GroupVersionKind().Kind,
				o.GetNamespace(), o.GetName()))
			return true
		},
		CreateFunc: func(e event.CreateEvent) bool {
			object := e.Object
			o := e.Object
			logger.Info(fmt.Sprintf("Create kind: %s Info: %s/%s",
				object.GetObjectKind().GroupVersionKind().Kind,
				o.GetNamespace(), o.GetName()))
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

func snapshotHandler(o client.Object) []reconcile.Request {
	snapshotInstance := o.(*utilsv1alpha1.Snapshot)

	logger := klogr.New().WithValues(
		"objectMapper",
		"snapshotHandler",
		"snapshot",
		snapshotInstance.Name,
	)

	logger.V(logs.LogInfo).Info("reacting to Snapshot change")

	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name: snapshotInstance.Name,
			},
		},
	}
}
