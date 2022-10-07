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
	"github.com/robfig/cron"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/logs"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	// requeueAfter is how long to wait before checking again to see if snapshot has been collected
	requeueAfter = 20 * time.Second
)

func SnapshotReconciler(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := klogr.New()
	logger.V(logs.LogInfo).Info("Reconciling")

	accessInstance := utils.GetAccessInstance()

	snapshotInstance := &utilsv1alpha1.Snapshot{}
	if err := accessInstance.GetResource(ctx, req.NamespacedName, snapshotInstance); err != nil {
		logger.Error(err, "unable to fetch Snapshot")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !snapshotInstance.DeletionTimestamp.IsZero() {
		return reconcileDelete(ctx, snapshotInstance, logger)
	}

	return reconcileNormal(ctx, snapshotInstance, logger)
}

func reconcileDelete(ctx context.Context, snapshotInstance *utilsv1alpha1.Snapshot,
	logger logr.Logger) (reconcile.Result, error) {

	snapshotClient := snapshotter.GetClient()
	err := snapshotClient.CleanupEntries(snapshotInstance)
	if err != nil {
		return ctrl.Result{}, err
	}

	if controllerutil.ContainsFinalizer(snapshotInstance, utilsv1alpha1.SnapshotFinalizer) {
		controllerutil.RemoveFinalizer(snapshotInstance, utilsv1alpha1.SnapshotFinalizer)
	}

	accessInstance := utils.GetAccessInstance()
	err = accessInstance.UpdateResource(ctx, snapshotInstance)

	logger.V(logs.LogInfo).Info("reconcileDelete succeeded")

	return ctrl.Result{}, err
}

func reconcileNormal(ctx context.Context, snapshotInstance *utilsv1alpha1.Snapshot,
	logger logr.Logger) (reconcile.Result, error) {

	accessInstance := utils.GetAccessInstance()
	if !controllerutil.ContainsFinalizer(snapshotInstance, utilsv1alpha1.SnapshotFinalizer) {
		if err := addFinalizer(ctx, snapshotInstance); err != nil {
			return reconcile.Result{}, err
		}
	}

	now := time.Now()
	nextRun, err := getNextScheduleTime(snapshotInstance, now)
	if err != nil {
		logger.V(logs.LogInfo).Info("failed to get next run. Err: %v", err)
		return ctrl.Result{}, err
	}

	logger.V(logs.LogInfo).Info(fmt.Sprintf("next run: %v", nextRun))

	snapshotClient := snapshotter.GetClient()
	// Get result, if any, from previous run
	result := snapshotClient.GetResult(ctx, snapshotInstance.Name)
	updateSnaphotStatus(snapshotInstance, result)

	if snapshotInstance.Status.NextScheduleTime == nil {
		logger.V(logs.LogInfo).Info("set NextScheduleTime")
		snapshotInstance.Status.NextScheduleTime = &metav1.Time{Time: *nextRun}
	} else {
		nextScheduledTime := *snapshotInstance.Status.NextScheduleTime
		if shouldSchedule(snapshotInstance, nextScheduledTime, logger) {
			snapshotClient := snapshotter.GetClient()
			logger.V(logs.LogInfo).Info("queuing snapshot job")
			err = snapshotClient.Collect(ctx, snapshotInstance.Name)
			if err != nil {
				return reconcile.Result{}, err
			}
			snapshotInstance.Status.LastRunTime = &metav1.Time{Time: now}
		}

		snapshotInstance.Status.NextScheduleTime = &metav1.Time{Time: *nextRun}
	}

	logger.V(logs.LogInfo).Info("patching Snapshot instance")
	err = accessInstance.UpdateResourceStatus(ctx, snapshotInstance)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to patch. Err: %v", err))
		return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}
	if isCollectionInProgress(snapshotInstance) {
		logger.V(logs.LogInfo).Info("snapshot collection still in progress")
		return reconcile.Result{Requeue: true, RequeueAfter: requeueAfter}, nil
	}

	logger.V(logs.LogInfo).Info("reconcile succeeded")
	scheduledResult := ctrl.Result{RequeueAfter: nextRun.Sub(now)}
	return scheduledResult, nil
}

func isCollectionInProgress(snapshotInstance *utilsv1alpha1.Snapshot) bool {
	return snapshotInstance.Status.LastRunStatus != nil &&
		*snapshotInstance.Status.LastRunStatus == utilsv1alpha1.SnapshotRunStatusInProgress
}

func updateSnaphotStatus(snapshotInstance *utilsv1alpha1.Snapshot, result snapshotter.Result) {
	var status utilsv1alpha1.SnapshotRunStatus
	var message string
	switch result.ResultStatus {
	case snapshotter.Collected:
		status = utilsv1alpha1.SnapshotRunStatusCollected
	case snapshotter.InProgress:
		status = utilsv1alpha1.SnapshotRunStatusInProgress
	case snapshotter.Failed:
		status = utilsv1alpha1.SnapshotRunStatusFailed
		message = result.Err.Error()
	case snapshotter.Unavailable:
		return
	}

	snapshotInstance.Status.LastRunStatus = &status
	snapshotInstance.Status.FailureMessage = &message
}

// getNextScheduleTime gets the time of next schedule after last scheduled and before now
func getNextScheduleTime(snapshot *utilsv1alpha1.Snapshot, now time.Time) (*time.Time, error) {
	sched, err := cron.ParseStandard(snapshot.Spec.Schedule)
	if err != nil {
		return nil, fmt.Errorf("unparseable schedule %q: %w", snapshot.Spec.Schedule, err)
	}

	var earliestTime time.Time
	if snapshot.Status.LastRunTime != nil {
		earliestTime = snapshot.Status.LastRunTime.Time
	} else {
		// If none found, then this is a recently created snapshot
		earliestTime = snapshot.ObjectMeta.CreationTimestamp.Time
	}
	if snapshot.Spec.StartingDeadlineSeconds != nil {
		// controller is not going to schedule anything below this point
		schedulingDeadline := now.Add(-time.Second * time.Duration(*snapshot.Spec.StartingDeadlineSeconds))

		if schedulingDeadline.After(earliestTime) {
			earliestTime = schedulingDeadline
		}
	}

	starts := 0
	for t := sched.Next(earliestTime); t.Before(now); t = sched.Next(t) {
		const maxNumberOfFailures = 100
		starts++
		if starts > maxNumberOfFailures {
			return nil,
				fmt.Errorf("too many missed start times (> %d). Set or decrease .spec.startingDeadlineSeconds or check clock skew",
					maxNumberOfFailures)
		}
	}

	next := sched.Next(now)
	return &next, nil
}

func shouldSchedule(snapshotInstance *utilsv1alpha1.Snapshot, nextScheduledTime metav1.Time, logger logr.Logger) bool {
	now := time.Now()
	logger.V(logs.LogInfo).Info(fmt.Sprintf("currently next schedule is %s", nextScheduledTime.Time))

	if now.Before(nextScheduledTime.Time) {
		logger.V(logs.LogInfo).Info("do not schedule yet")
		return false
	}

	// if last processed request was within 30 seconds, ignore it.
	// Avoid reprocessing spuriors back-to-back reconciliations
	if snapshotInstance.Status.LastRunTime != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("last snapshot was requested at %s", snapshotInstance.Status.LastRunTime))
		const ignoreTimeInSecond = 30
		diff := now.Sub(snapshotInstance.Status.LastRunTime.Time)
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Elapsed time since last snapshot in minutes %f",
			diff.Minutes()))
		return diff.Seconds() >= ignoreTimeInSecond
	}

	return true
}

func addFinalizer(ctx context.Context, snapshotInstance *utilsv1alpha1.Snapshot) error {
	controllerutil.AddFinalizer(snapshotInstance, utilsv1alpha1.SnapshotFinalizer)

	accessInstance := utils.GetAccessInstance()
	err := accessInstance.UpdateResource(ctx, snapshotInstance)
	if err != nil {
		return err
	}

	return accessInstance.GetResource(ctx,
		types.NamespacedName{Name: snapshotInstance.Name}, snapshotInstance)
}
