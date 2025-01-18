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
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func SnapshotReconciler(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
	logger.V(logs.LogInfo).Info("Reconciling")

	accessInstance := utils.GetAccessInstance()

	snapshotInstance := &utilsv1beta1.Snapshot{}
	if err := accessInstance.GetResource(ctx, req.NamespacedName, snapshotInstance); err != nil {
		logger.Error(err, "unable to fetch Snapshot")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger = logger.WithValues("snapshot", snapshotInstance.Name)

	if !snapshotInstance.DeletionTimestamp.IsZero() {
		return reconcileDelete(ctx, snapshotInstance, collector.Snapshot, snapshotInstance.Spec.Storage,
			utilsv1alpha1.SnapshotFinalizer, logger)
	}

	return reconcileSnapshotNormal(ctx, snapshotInstance, logger)
}

func reconcileSnapshotNormal(ctx context.Context, snapshotInstance *utilsv1alpha1.Snapshot,
	logger logr.Logger) (reconcile.Result, error) {

	logger.V(logs.LogInfo).Info("reconcileSnapshotNormal")
	if err := addFinalizer(ctx, snapshotInstance, utilsv1alpha1.SnapshotFinalizer); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to add finalizer: %s", err))
		return reconcile.Result{}, err
	}

	collectionSnapshot := collectionSnapshot{snapshotInstance: snapshotInstance}
	snapshotClient := collector.GetClient()
	// Get result, if any, from previous run
	result := snapshotClient.GetResult(ctx, snapshotInstance.Name, collector.Snapshot)
	updateStatus(result, &collectionSnapshot)

	now := time.Now()
	nextRun, err := schedule(ctx, snapshotInstance, collector.Snapshot,
		collectSnapshot, &collectionSnapshot, logger)
	if err != nil {
		logger.V(logs.LogInfo).Info("failed to get next run. Err: %v", err)
		return ctrl.Result{}, err
	}

	snapshotInstance = collectionSnapshot.snapshotInstance

	logger.V(logs.LogInfo).Info("patching Snapshot instance")
	err = utils.GetAccessInstance().UpdateResourceStatus(ctx, snapshotInstance)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to patch. Err: %v", err))
		return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}
	if isCollectionInProgress(snapshotInstance.Status.LastRunStatus) {
		logger.V(logs.LogInfo).Info("snapshot collection still in progress")
		return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}

	logger.V(logs.LogInfo).Info("reconcile snapshot succeeded")
	scheduledResult := ctrl.Result{RequeueAfter: nextRun.Sub(now)}
	return scheduledResult, nil
}
