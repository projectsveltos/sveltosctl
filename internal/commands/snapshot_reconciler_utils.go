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
	"fmt"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

type collectionSnapshot struct {
	snapshotInstance *utilsv1alpha1.Snapshot
}

func (c *collectionSnapshot) getCreationTimestamp() *metav1.Time {
	return &c.snapshotInstance.CreationTimestamp
}

func (c *collectionSnapshot) getSchedule() string {
	return c.snapshotInstance.Spec.Schedule
}

func (c *collectionSnapshot) getNextScheduleTime() *metav1.Time {
	return c.snapshotInstance.Status.NextScheduleTime
}

func (c *collectionSnapshot) setNextScheduleTime(t *metav1.Time) {
	c.snapshotInstance.Status.NextScheduleTime = t
}

func (c *collectionSnapshot) getLastRunTime() *metav1.Time {
	return c.snapshotInstance.Status.LastRunTime
}

func (c *collectionSnapshot) setLastRunTime(t *metav1.Time) {
	c.snapshotInstance.Status.LastRunTime = t
}

func (c *collectionSnapshot) getStartingDeadlineSeconds() *int64 {
	return c.snapshotInstance.Spec.StartingDeadlineSeconds
}

func (c *collectionSnapshot) setLastRunStatus(s utilsv1alpha1.CollectionStatus) {
	c.snapshotInstance.Status.LastRunStatus = &s
}

func (c *collectionSnapshot) setFailureMessage(m string) {
	c.snapshotInstance.Status.FailureMessage = &m
}

func collectSnapshot(ctx context.Context, c client.Client, snapshotName string, logger logr.Logger) error {
	logger = logger.WithValues("snapshot", snapshotName)
	logger.V(logs.LogInfo).Info("collect snapshot")

	// Get Snapshot instance
	snapshotInstance := &utilsv1alpha1.Snapshot{}
	err := c.Get(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Snapshot %s does not exist anymore. Nothing to do.", snapshotName))
		}
	}

	collectorClient := collector.GetClient()

	if snapshotInstance.Spec.SuccessfulSnapshotLimit != nil {
		err = collectorClient.CleanOldCollections(snapshotInstance.Spec.Storage, snapshotInstance.Name, collector.Snapshot,
			*snapshotInstance.Spec.SuccessfulSnapshotLimit, logger)
		if err != nil {
			return err
		}
	}

	now := time.Now()
	folder := collectorClient.GetFolderPath(snapshotInstance.Spec.Storage, snapshotInstance.Name, collector.Snapshot, now)

	// Collect all ClusterProfiles
	err = dumpClusterProfiles(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClusterConfigurations(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClusters(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpClassifiers(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpRoleRequests(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpEventSources(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	err = dumpEventBasedAddOns(collectorClient, ctx, folder, logger)
	if err != nil {
		return err
	}
	return nil
}

func dumpEventSources(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing EventSources")
	eventSources, err := utils.GetAccessInstance().ListEventSources(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d EventSources", len(eventSources.Items)))
	for i := range eventSources.Items {
		rr := &eventSources.Items[i]
		err = collectorClient.DumpObject(rr, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpEventBasedAddOns(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing EventBasedAddOns")
	eventBasedAddOns, err := utils.GetAccessInstance().ListEventBasedAddOns(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d EventBasedAddOns", len(eventBasedAddOns.Items)))
	for i := range eventBasedAddOns.Items {
		r := &eventBasedAddOns.Items[i]
		err = collectorClient.DumpObject(r, folder, logger)
		if err != nil {
			return err
		}
		err = dumpReferencedObjects(collectorClient, ctx, r.Spec.PolicyRefs, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpRoleRequests(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing RoleRequests")
	roleRequests, err := utils.GetAccessInstance().ListRoleRequests(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d RoleRequests", len(roleRequests.Items)))
	for i := range roleRequests.Items {
		rr := &roleRequests.Items[i]
		err = collectorClient.DumpObject(rr, folder, logger)
		if err != nil {
			return err
		}
		err = dumpReferencedObjects(collectorClient, ctx, rr.Spec.RoleRefs, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpClassifiers(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing Classifiers")
	classifiers, err := utils.GetAccessInstance().ListClassifiers(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Classifiers", len(classifiers.Items)))
	for i := range classifiers.Items {
		cl := &classifiers.Items[i]
		err = collectorClient.DumpObject(cl, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpClusterProfiles(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing ClusterProfiles")
	clusterProfiles, err := utils.GetAccessInstance().ListClusterProfiles(ctx, logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d ClusterProfiles", len(clusterProfiles.Items)))
	for i := range clusterProfiles.Items {
		cc := &clusterProfiles.Items[i]
		err = collectorClient.DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
		err = dumpReferencedObjects(collectorClient, ctx, cc.Spec.PolicyRefs, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpReferencedObjects(collectorClient *collector.Collector, ctx context.Context, referencedObjects []libsveltosv1alpha1.PolicyRef,
	folder string, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info("storing ClusterProfiles's referenced resources")
	var object client.Object
	for i := range referencedObjects {
		ref := &referencedObjects[i]
		if ref.Kind == string(libsveltosv1alpha1.ConfigMapReferencedResourceKind) {
			configMap := &corev1.ConfigMap{}
			err := utils.GetAccessInstance().GetResource(ctx,
				types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, configMap)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Found referenced ConfigMap %s/%s", configMap.Namespace, configMap.Name))
			object = configMap
		} else {
			// TODO: Allow certain Secret to be skipped
			secret := &corev1.Secret{}
			err := utils.GetAccessInstance().GetResource(ctx,
				types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, secret)
			if err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return err
			}
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Found referenced Secret %s/%s", secret.Namespace, secret.Name))
			object = secret
		}

		if err := collectorClient.DumpObject(object, folder, logger); err != nil {
			return err
		}
	}

	return nil
}

func dumpClusterConfigurations(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing ClusterConfigurations")
	clusterConfigurations, err := utils.GetAccessInstance().ListClusterConfigurations(ctx, "", logger)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d ClusterConfigurations", len(clusterConfigurations.Items)))
	for i := range clusterConfigurations.Items {
		cc := &clusterConfigurations.Items[i]
		err = collectorClient.DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpCAPIClusters(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing CAPI Clusters")
	clusterList := &clusterv1.ClusterList{}
	err := utils.GetAccessInstance().ListResources(ctx, clusterList)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Clusters", len(clusterList.Items)))
	for i := range clusterList.Items {
		cc := &clusterList.Items[i]
		err = collectorClient.DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpSveltosClusters(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info("storing Sveltos Clusters")
	clusterList := &libsveltosv1alpha1.SveltosClusterList{}
	err := utils.GetAccessInstance().ListResources(ctx, clusterList)
	if err != nil {
		return err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d Clusters", len(clusterList.Items)))
	for i := range clusterList.Items {
		cc := &clusterList.Items[i]
		err = collectorClient.DumpObject(cc, folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func dumpClusters(collectorClient *collector.Collector, ctx context.Context, folder string, logger logr.Logger) error {
	if err := dumpCAPIClusters(collectorClient, ctx, folder, logger); err != nil {
		return err
	}

	if err := dumpSveltosClusters(collectorClient, ctx, folder, logger); err != nil {
		return err
	}

	return nil
}

func updateSnaphotPredicate(e event.UpdateEvent) bool {
	newObject := e.ObjectNew.(*utilsv1alpha1.Snapshot)
	oldObject := e.ObjectOld.(*utilsv1alpha1.Snapshot)

	if oldObject == nil ||
		!reflect.DeepEqual(newObject.Spec, oldObject.Spec) {

		return true
	}

	return false
}
