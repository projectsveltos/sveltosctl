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
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/crd"
	"github.com/projectsveltos/libsveltos/lib/logsettings"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	libsveltosset "github.com/projectsveltos/libsveltos/lib/set"
	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

type collection interface {
	getCreationTimestamp() *metav1.Time

	getSchedule() string

	getNextScheduleTime() *metav1.Time

	setNextScheduleTime(*metav1.Time)

	getLastRunTime() *metav1.Time

	setLastRunTime(*metav1.Time)

	getStartingDeadlineSeconds() *int64

	setLastRunStatus(utilsv1beta1.CollectionStatus)

	setFailureMessage(string)
}

const (
	// requeueAfter is how long to wait before checking again to see if snapshot has been collected
	requeueAfter = 20 * time.Second
)

var (
	mux sync.Mutex

	// key: Sveltos/CAPI Cluster; value: set of all Techsupport matching the Cluster
	clusterMap map[corev1.ObjectReference]*libsveltosset.Set

	// key: Techsupport; value: set of Sveltos/CAPI Clusters matched
	techsupportMap map[corev1.ObjectReference]*libsveltosset.Set

	// key: techsupport; value techsupport's Selector
	techsupports map[corev1.ObjectReference]libsveltosv1beta1.Selector
)

func watchResources(ctx context.Context, logger logr.Logger) error {
	mux = sync.Mutex{}
	clusterMap = make(map[corev1.ObjectReference]*libsveltosset.Set)
	techsupportMap = make(map[corev1.ObjectReference]*libsveltosset.Set)
	techsupports = make(map[corev1.ObjectReference]libsveltosv1beta1.Selector)

	scheme, _ := utils.GetScheme()
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:         scheme,
		LeaderElection: false,
	})
	if err != nil {
		logger.Error(err, "unable to start manager")
		os.Exit(1)
	}

	const workerNumber = 10
	collector.InitializeClient(ctx, logger.WithName("collector"), mgr.GetClient(),
		workerNumber)

	err = startSnapshotReconciler(ctx, mgr, logger)
	if err != nil {
		logger.Error(err, "failed to start snapshot reconciler")
	}

	var techsupportController controller.Controller
	techsupportController, err = startTechsupportReconciler(ctx, mgr, logger)
	if err != nil {
		logger.Error(err, "failed to start snapshot reconciler")
	}

	go capiWatchers(ctx, mgr, techsupportController, logger)

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "unable to continue running manager")
		return err
	}

	return nil
}

func startSnapshotReconciler(ctx context.Context, mgr manager.Manager, logger logr.Logger) error {
	// Create an un-managed controller
	c, err := controller.NewUnmanaged("snapshot-watcher", mgr, controller.Options{
		Reconciler:              reconcile.Func(SnapshotReconciler),
		MaxConcurrentReconciles: 1,
	})

	if err != nil {
		logger.Error(err, "unable to create snapshot watcher")
		return err
	}

	sourceSnapshot := source.Kind[*utilsv1beta1.Snapshot](
		mgr.GetCache(),
		&utilsv1beta1.Snapshot{},
		handler.TypedEnqueueRequestsFromMapFunc(handlerSnapshotMapFun),
		SnapshotPredicate{Logger: mgr.GetLogger().WithValues("predicate", "clusterpredicate")},
	)

	if err := c.Watch(sourceSnapshot); err != nil {
		return err
	}

	// Start controller in a goroutine so not to block.
	go func() {
		// Start controller. This will block until the context is
		// closed, or the controller returns an error.
		logger.Info("Starting watcher controller")
		if err := c.Start(ctx); err != nil {
			logger.Error(err, "cannot run controller")
			panic(1)
		}
	}()

	return nil
}

func startTechsupportReconciler(ctx context.Context, mgr manager.Manager, logger logr.Logger) (controller.Controller, error) {
	// Create an un-managed controller
	c, err := controller.NewUnmanaged("techsupport-watcher", mgr, controller.Options{
		Reconciler:              reconcile.Func(TechsupportReconciler),
		MaxConcurrentReconciles: 1,
	})

	if err != nil {
		logger.Error(err, "unable to create techsupport watcher")
		return nil, err
	}

	sourceTechsupport := source.Kind[*utilsv1beta1.Techsupport](
		mgr.GetCache(),
		&utilsv1beta1.Techsupport{},
		handler.TypedEnqueueRequestsFromMapFunc(handlerTechsupportMapFun),
		TechsupportPredicate{Logger: mgr.GetLogger().WithValues("predicate", "techsupportpredicate")},
	)

	if err := c.Watch(sourceTechsupport); err != nil {
		return nil, err
	}

	sourceSveltosCluster := source.Kind[*libsveltosv1beta1.SveltosCluster](
		mgr.GetCache(),
		&libsveltosv1beta1.SveltosCluster{},
		handler.TypedEnqueueRequestsFromMapFunc(requeueTechsupportForSveltosCluster),
		SveltosClusterPredicate{Logger: mgr.GetLogger().WithValues("predicate", "sveltosclusterpredicate")},
	)

	if err := c.Watch(sourceSveltosCluster); err != nil {
		return nil, err
	}

	// Start controller in a goroutine so not to block.
	go func() {
		// Start controller. This will block until the context is
		// closed, or the controller returns an error.
		logger.Info("Starting watcher controller")
		if err := c.Start(ctx); err != nil {
			logger.Error(err, "cannot run controller")
			panic(1)
		}
	}()

	return c, nil
}

func watchForCAPI(mgr ctrl.Manager, c controller.Controller) error {
	sourceCluster := source.Kind[*clusterv1.Cluster](
		mgr.GetCache(),
		&clusterv1.Cluster{},
		handler.TypedEnqueueRequestsFromMapFunc(requeueTechsupportForCluster),
		ClusterPredicate{Logger: mgr.GetLogger().WithValues("predicate", "clusterpredicate")},
	)

	// When cluster-api cluster changes, according to ClusterPredicates,
	// one or more ClusterProfiles need to be reconciled.
	if err := c.Watch(sourceCluster); err != nil {
		return err
	}

	return nil
}

// getNextScheduleTime gets the time of next schedule after last scheduled and before now
func getNextScheduleTime(collectionInstance collection, now time.Time) (*time.Time, error) {
	sched, err := cron.ParseStandard(collectionInstance.getSchedule())
	if err != nil {
		return nil, fmt.Errorf("unparseable schedule %q: %w", collectionInstance.getSchedule(), err)
	}

	var earliestTime time.Time
	if collectionInstance.getLastRunTime() != nil {
		earliestTime = collectionInstance.getLastRunTime().Time
	} else {
		// If none found, then this is a recently created snapshot
		earliestTime = collectionInstance.getCreationTimestamp().Time
	}
	if collectionInstance.getStartingDeadlineSeconds() != nil {
		// controller is not going to schedule anything below this point
		schedulingDeadline := now.Add(-time.Second * time.Duration(*collectionInstance.getStartingDeadlineSeconds()))

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

func shouldSchedule(collectionInstance collection, logger logr.Logger) bool {
	now := time.Now()
	logger.V(logs.LogInfo).Info(fmt.Sprintf("currently next schedule is %s", collectionInstance.getNextScheduleTime().Time))

	if now.Before(collectionInstance.getNextScheduleTime().Time) {
		logger.V(logs.LogInfo).Info("do not schedule yet")
		return false
	}

	// if last processed request was within 30 seconds, ignore it.
	// Avoid reprocessing spuriors back-to-back reconciliations
	if collectionInstance.getLastRunTime() != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("last snapshot was requested at %s", collectionInstance.getLastRunTime()))
		const ignoreTimeInSecond = 30
		diff := now.Sub(collectionInstance.getLastRunTime().Time)
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Elapsed time since last snapshot in minutes %f",
			diff.Minutes()))
		return diff.Seconds() >= ignoreTimeInSecond
	}

	return true
}

func addFinalizer(ctx context.Context, instance client.Object, finalizer string) error {
	if controllerutil.ContainsFinalizer(instance, finalizer) {
		return nil
	}

	controllerutil.AddFinalizer(instance, finalizer)
	accessInstance := utils.GetAccessInstance()
	err := accessInstance.UpdateResource(ctx, instance)
	if err != nil {
		return err
	}

	return accessInstance.GetResource(ctx,
		types.NamespacedName{Name: instance.GetName()}, instance)
}

func handlerSnapshotMapFun(ctx context.Context, snapshot *utilsv1beta1.Snapshot) []reconcile.Request {
	return handlerMapFun(snapshot)
}

func handlerTechsupportMapFun(ctx context.Context, techsupport *utilsv1beta1.Techsupport) []reconcile.Request {
	return handlerMapFun(techsupport)
}

func handlerMapFun(o client.Object) []reconcile.Request {
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))).WithValues(
		"objectMapper",
		"handler",
		"instance",
		o.GetName(),
	)

	logger.V(logs.LogInfo).Info("reacting to instance change")

	return []reconcile.Request{
		{
			NamespacedName: client.ObjectKey{
				Name: o.GetName(),
			},
		},
	}
}

func updateStatus(result collector.Result, collectionInstance collection) {
	var status utilsv1beta1.CollectionStatus
	var message string

	switch result.ResultStatus {
	case collector.Collected:
		status = utilsv1beta1.CollectionStatusCollected
	case collector.InProgress:
		status = utilsv1beta1.CollectionStatusInProgress
	case collector.Failed:
		status = utilsv1beta1.CollectionStatusFailed
		message = result.Err.Error()
	case collector.Unavailable:
		return
	}

	collectionInstance.setLastRunStatus(status)
	collectionInstance.setFailureMessage(message)
}

func isCollectionInProgress(lastRunStatus *utilsv1beta1.CollectionStatus) bool {
	return lastRunStatus != nil &&
		*lastRunStatus == utilsv1beta1.CollectionStatusInProgress
}

func removeQueuedJobsAndFinalizer(c *collector.Collector, instance client.Object, collectionType collector.CollectionType,
	storage, finalizer string, logger logr.Logger) error {

	err := c.CleanupEntries(storage, instance.GetName(), collectionType)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to cleanup: %s", err))
		return err
	}

	if controllerutil.ContainsFinalizer(instance, finalizer) {
		controllerutil.RemoveFinalizer(instance, finalizer)
	}

	return nil
}

func reconcileDelete(ctx context.Context, instance client.Object, collectionType collector.CollectionType,
	storage, finalizer string, logger logr.Logger) (reconcile.Result, error) {

	logger.V(logs.LogInfo).Info("reconcileDelete")

	err := removeQueuedJobsAndFinalizer(collector.GetClient(), instance, collectionType, storage, finalizer, logger)
	if err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to cleanup: %s", err))
		return ctrl.Result{}, err
	}

	err = utils.GetAccessInstance().UpdateResource(ctx, instance)
	if err != nil {
		return ctrl.Result{}, err
	}

	logger.V(logs.LogInfo).Info("reconcileDelete succeeded")
	return ctrl.Result{}, nil
}

type SnapshotPredicate struct {
	Logger logr.Logger
}

func (p SnapshotPredicate) Create(obj event.TypedCreateEvent[*utilsv1beta1.Snapshot]) bool {
	o := obj.Object
	p.Logger.Info(fmt.Sprintf("Create kind: %s Info: %s/%s",
		o.GetObjectKind().GroupVersionKind().Kind,
		o.GetNamespace(), o.GetName()))
	return true
}

func (p SnapshotPredicate) Update(obj event.TypedUpdateEvent[*utilsv1beta1.Snapshot]) bool {
	return updateSnaphotPredicate(obj.ObjectNew, obj.ObjectOld)
}

func (p SnapshotPredicate) Delete(obj event.TypedDeleteEvent[*utilsv1beta1.Snapshot]) bool {
	o := obj.Object
	p.Logger.Info(fmt.Sprintf("Delete kind: %s Info: %s/%s",
		o.GetObjectKind().GroupVersionKind().Kind,
		o.GetNamespace(), o.GetName()))
	return true
}

func (p SnapshotPredicate) Generic(obj event.TypedGenericEvent[*utilsv1beta1.Snapshot]) bool {
	return false
}

type TechsupportPredicate struct {
	Logger logr.Logger
}

func (p TechsupportPredicate) Create(obj event.TypedCreateEvent[*utilsv1beta1.Techsupport]) bool {
	o := obj.Object
	p.Logger.Info(fmt.Sprintf("Create kind: %s Info: %s/%s",
		o.GetObjectKind().GroupVersionKind().Kind,
		o.GetNamespace(), o.GetName()))
	return true
}

func (p TechsupportPredicate) Update(obj event.TypedUpdateEvent[*utilsv1beta1.Techsupport]) bool {
	return updateTechsupportPredicate(obj.ObjectNew, obj.ObjectOld)
}

func (p TechsupportPredicate) Delete(obj event.TypedDeleteEvent[*utilsv1beta1.Techsupport]) bool {
	o := obj.Object
	p.Logger.Info(fmt.Sprintf("Delete kind: %s Info: %s/%s",
		o.GetObjectKind().GroupVersionKind().Kind,
		o.GetNamespace(), o.GetName()))
	return true
}

func (p TechsupportPredicate) Generic(obj event.TypedGenericEvent[*utilsv1beta1.Techsupport]) bool {
	return false
}

func schedule(ctx context.Context, instance client.Object, collectionType collector.CollectionType, collectMethod collector.CollectMethod,
	collectionInstance collection, logger logr.Logger) (*time.Time, error) {

	newLastRunTime := collectionInstance.getLastRunTime()

	now := time.Now()
	nextRun, err := getNextScheduleTime(collectionInstance, now)
	if err != nil {
		logger.V(logs.LogInfo).Info("failed to get next run. Err: %v", err)
		return nil, err
	}

	var newNextScheduleTime *metav1.Time
	c := collector.GetClient()
	if collectionInstance.getNextScheduleTime() == nil {
		logger.V(logs.LogInfo).Info("set NextScheduleTime")
		newNextScheduleTime = &metav1.Time{Time: *nextRun}
	} else {
		if shouldSchedule(collectionInstance, logger) {
			logger.V(logs.LogInfo).Info("queuing collection job")
			err := c.Collect(ctx, instance.GetName(), collectionType, collectMethod)
			if err != nil {
				return nil, err
			}
			newLastRunTime = &metav1.Time{Time: now}
		}

		newNextScheduleTime = &metav1.Time{Time: *nextRun}
	}

	collectionInstance.setLastRunTime(newLastRunTime)
	collectionInstance.setNextScheduleTime(newNextScheduleTime)

	return nextRun, nil
}

// isCAPIInstalled returns true if CAPI is installed, false otherwise
func isCAPIInstalled(ctx context.Context, c client.Client) (bool, error) {
	clusterCRD := &apiextensionsv1.CustomResourceDefinition{}

	err := c.Get(ctx, types.NamespacedName{Name: "clusters.cluster.x-k8s.io"}, clusterCRD)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

func capiWatchers(ctx context.Context, mgr ctrl.Manager, techsupportController controller.Controller,
	logger logr.Logger) {

	const maxRetries = 20
	retries := 0
	for {
		capiPresent, err := isCAPIInstalled(ctx, mgr.GetClient())
		if err != nil {
			if retries < maxRetries {
				logger.Info(fmt.Sprintf("failed to verify if CAPI is present: %v", err))
				time.Sleep(time.Second)
			}
			retries++
		} else {
			if !capiPresent {
				logger.V(logs.LogDebug).Info("CAPI currently not present. Starting CRD watcher")
				go crd.WatchCustomResourceDefinition(ctx, mgr.GetConfig(), capiCRDHandler, logger)
			} else {
				logger.V(logsettings.LogInfo).Info("CAPI present.")
				err = watchForCAPI(mgr, techsupportController)
				if err != nil {
					continue
				}
			}
			return
		}
	}
}

// capiCRDHandler restarts process if a CAPI CRD is updated
func capiCRDHandler(gvk *schema.GroupVersionKind) {
	if gvk.Group == clusterv1.GroupVersion.Group {
		if killErr := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); killErr != nil {
			panic("kill -TERM failed")
		}
	}
}
