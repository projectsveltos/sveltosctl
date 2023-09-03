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
	"path"
	"reflect"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

type collectionTechsupport struct {
	techsupportInstance *utilsv1alpha1.Techsupport
}

func (c *collectionTechsupport) getCreationTimestamp() *metav1.Time {
	return &c.techsupportInstance.CreationTimestamp
}

func (c *collectionTechsupport) getSchedule() string {
	return c.techsupportInstance.Spec.Schedule
}

func (c *collectionTechsupport) getNextScheduleTime() *metav1.Time {
	return c.techsupportInstance.Status.NextScheduleTime
}

func (c *collectionTechsupport) setNextScheduleTime(t *metav1.Time) {
	c.techsupportInstance.Status.NextScheduleTime = t
}

func (c *collectionTechsupport) getLastRunTime() *metav1.Time {
	return c.techsupportInstance.Status.LastRunTime
}

func (c *collectionTechsupport) setLastRunTime(t *metav1.Time) {
	c.techsupportInstance.Status.LastRunTime = t
}

func (c *collectionTechsupport) getStartingDeadlineSeconds() *int64 {
	return c.techsupportInstance.Spec.StartingDeadlineSeconds
}

func (c *collectionTechsupport) setLastRunStatus(s utilsv1alpha1.CollectionStatus) {
	c.techsupportInstance.Status.LastRunStatus = &s
}

func (c *collectionTechsupport) setFailureMessage(m string) {
	c.techsupportInstance.Status.FailureMessage = &m
}

func collectTechsupport(ctx context.Context, c client.Client, techsupportName string,
	logger logr.Logger) error {

	logger = logger.WithValues("techsupport", techsupportName)
	logger.V(logs.LogInfo).Info("collect techsupport")

	techsupportInstance := &utilsv1alpha1.Techsupport{}
	err := c.Get(ctx, types.NamespacedName{Name: techsupportName}, techsupportInstance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(
				fmt.Sprintf("Techsupport %s does not exist anymore. Nothing to do.", techsupportName))
			return nil
		}

		return err
	}

	collectorClient := collector.GetClient()

	if techsupportInstance.Spec.SuccessfulTechsupportLimit != nil {
		err = collectorClient.CleanOldCollections(techsupportInstance.Spec.Storage, techsupportInstance.Name,
			collector.Techsupport, *techsupportInstance.Spec.SuccessfulTechsupportLimit, logger)
		if err != nil {
			return err
		}
	}

	now := time.Now()
	folder := collectorClient.GetFolderPath(techsupportInstance.Spec.Storage, techsupportInstance.Name,
		collector.Techsupport, now)

	err = nil
	for i := range techsupportInstance.Status.MatchingClusterRefs {
		cluster := &techsupportInstance.Status.MatchingClusterRefs[i]
		l := logger.WithValues("cluster", fmt.Sprintf("%s:%s/%s", clusterproxy.GetClusterType(cluster),
			cluster.Namespace, cluster.Name))
		clusterFolder := path.Join(folder, fmt.Sprintf("%s:%s/%s", clusterproxy.GetClusterType(cluster),
			cluster.Namespace, cluster.Name))
		if tmpErr := collectTechsupportForCluster(ctx, c, techsupportInstance, cluster, clusterFolder, l); tmpErr != nil {
			l.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect techsupport: %v", tmpErr))
			if err == nil {
				err = tmpErr
			} else {
				err = errors.Wrap(err, tmpErr.Error())
			}
		}
	}

	if techsupportInstance.Spec.Tar {
		if tmpErr := collector.GetClient().TarDir(folder, logger); tmpErr != nil {
			if err == nil {
				err = tmpErr
			} else {
				err = errors.Wrap(err, tmpErr.Error())
			}
		}
	}

	if err != nil {
		logger.V(logs.LogInfo).Info("done collecting techsupport")
	}

	return err
}

func collectTechsupportForCluster(ctx context.Context, c client.Client, techsupportInstance *utilsv1alpha1.Techsupport,
	cluster *corev1.ObjectReference, folder string, logger logr.Logger) error {

	ready, err := clusterproxy.IsClusterReadyToBeConfigured(ctx, c, cluster, logger)
	if err != nil {
		return err
	}
	if !ready {
		logger.V(logs.LogInfo).Info("Cluster is not ready yet")
		return nil
	}
	logger.V(logs.LogInfo).Info("collecting techsupport")

	saNamespace, saName := getClusterSummaryServiceAccountInfo(techsupportInstance)

	remoteRestConfig, err := clusterproxy.GetKubernetesRestConfig(ctx, utils.GetAccessInstance().GetClient(),
		cluster.Namespace, cluster.Name, saNamespace, saName, clusterproxy.GetClusterType(cluster), logger)
	if err != nil {
		return err
	}

	remoteClientSet, err := kubernetes.NewForConfig(remoteRestConfig)
	if err != nil {
		return err
	}

	remoteClient, err := clusterproxy.GetKubernetesClient(ctx, utils.GetAccessInstance().GetClient(),
		cluster.Namespace, cluster.Name, saNamespace, saName, clusterproxy.GetClusterType(cluster), logger)
	if err != nil {
		return err
	}

	err = nil
	for i := range techsupportInstance.Spec.Logs {
		tmpErr := collectLogs(ctx, remoteClientSet, remoteClient, &techsupportInstance.Spec.Logs[i], folder, logger)
		if tmpErr != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to collect logs %v", err))
			if err == nil {
				err = tmpErr
			} else {
				err = errors.Wrap(err, tmpErr.Error())
			}
		}
	}

	for i := range techsupportInstance.Spec.Resources {
		resourceFolder := path.Join(folder, "resources")
		tmpErr := dumpResources(ctx, remoteRestConfig, &techsupportInstance.Spec.Resources[i], resourceFolder, logger)
		if tmpErr != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to dump resources %v", err))
			if err == nil {
				err = tmpErr
			} else {
				err = errors.Wrap(err, tmpErr.Error())
			}
		}
	}

	return err
}

func dumpResources(ctx context.Context, remoteRestConfig *rest.Config, resource *utilsv1alpha1.Resource,
	folder string, logger logr.Logger) error {

	logger = logger.WithValues("gvk", fmt.Sprintf("%s:%s:%s", resource.Group, resource.Version, resource.Kind))
	logger.V(logs.LogInfo).Info("collecting resources")

	gvk := schema.GroupVersionKind{
		Group:   resource.Group,
		Version: resource.Version,
		Kind:    resource.Kind,
	}

	dc := discovery.NewDiscoveryClientForConfigOrDie(remoteRestConfig)
	groupResources, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return err
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)

	d := dynamic.NewForConfigOrDie(remoteRestConfig)

	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		if meta.IsNoMatchError(err) {
			return nil
		}
		return err
	}

	resourceId := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: mapping.Resource.Resource,
	}

	options := metav1.ListOptions{}

	if len(resource.LabelFilters) > 0 {
		labelFilter := ""
		for i := range resource.LabelFilters {
			if labelFilter != "" {
				labelFilter += ","
			}
			f := resource.LabelFilters[i]
			if f.Operation == libsveltosv1alpha1.OperationEqual {
				labelFilter += fmt.Sprintf("%s=%s", f.Key, f.Value)
			} else {
				labelFilter += fmt.Sprintf("%s!=%s", f.Key, f.Value)
			}
		}

		options.LabelSelector = labelFilter
	}

	if resource.Namespace != "" {
		if options.FieldSelector != "" {
			options.FieldSelector += ","
		}
		options.FieldSelector += fmt.Sprintf("metadata.namespace=%s", resource.Namespace)
	}

	list, err := d.Resource(resourceId).List(ctx, options)
	if err != nil {
		return err
	}

	logger.V(logs.LogInfo).Info(fmt.Sprintf("collected %d resources", len(list.Items)))
	for i := range list.Items {
		err = collector.GetClient().DumpObject(&list.Items[i], folder, logger)
		if err != nil {
			return err
		}
	}

	return nil
}

func collectLogs(ctx context.Context, remoteClientSet *kubernetes.Clientset, remoteClient client.Client,
	log *utilsv1alpha1.Log, folder string, logger logr.Logger) error {

	logger.V(logs.LogInfo).Info("collecting logs")
	options := client.ListOptions{}

	if len(log.LabelFilters) > 0 {
		labelFilter := ""
		for i := range log.LabelFilters {
			if labelFilter != "" {
				labelFilter += ","
			}
			f := log.LabelFilters[i]
			if f.Operation == libsveltosv1alpha1.OperationEqual {
				labelFilter += fmt.Sprintf("%s=%s", f.Key, f.Value)
			} else {
				labelFilter += fmt.Sprintf("%s!=%s", f.Key, f.Value)
			}
		}

		parsedSelector, err := labels.Parse(labelFilter)
		if err != nil {
			return err
		}
		options.LabelSelector = parsedSelector
	}

	if log.Namespace != "" {
		options.Namespace = log.Namespace
	}

	pods := &corev1.PodList{}
	if err := remoteClient.List(ctx, pods, &options); err != nil {
		return err
	}

	logger.V(logs.LogInfo).Info(fmt.Sprintf("found %d pods", len(pods.Items)))
	for i := range pods.Items {
		if err := collector.GetClient().DumpPodLogs(ctx, remoteClientSet, folder, log.SinceSeconds,
			&pods.Items[i]); err != nil {
			return err
		}
	}

	return nil
}

func updateTechsupportPredicate(newObject, oldObject *utilsv1alpha1.Techsupport) bool {
	if oldObject == nil ||
		!reflect.DeepEqual(newObject.Spec, oldObject.Spec) {

		return true
	}

	return false
}

func requeueTechsupportForSveltosCluster(
	ctx context.Context, sveltosCluster *libsveltosv1alpha1.SveltosCluster,
) []reconcile.Request {

	return requeueTechsupportForACluster(sveltosCluster)
}

func requeueTechsupportForCluster(
	ctx context.Context, cluster *clusterv1.Cluster,
) []reconcile.Request {

	return requeueTechsupportForACluster(cluster)
}

func requeueTechsupportForACluster(
	cluster client.Object,
) []reconcile.Request {

	mux.Lock()
	defer mux.Unlock()

	instance := utils.GetAccessInstance()
	addTypeInformationToObject(instance.GetScheme(), cluster)

	apiVersion, kind := cluster.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()

	clusterInfo := corev1.ObjectReference{APIVersion: apiVersion, Kind: kind,
		Namespace: cluster.GetNamespace(), Name: cluster.GetName()}

	// Get all Techsupport previously matching this cluster and reconcile those
	requests := make([]ctrl.Request, getClusterMapForEntry(&clusterInfo).Len())
	consumers := getClusterMapForEntry(&clusterInfo).Items()

	for i := range consumers {
		requests[i] = ctrl.Request{
			NamespacedName: client.ObjectKey{
				Name: consumers[i].Name,
			},
		}
	}

	// Iterate over all current ClusterProfile and reconcile the ClusterProfile now
	// matching the Cluster
	for k := range techsupports {
		techsupportSelector := techsupports[k]
		parsedSelector, err := labels.Parse(string(techsupportSelector))
		if err != nil {
			// When clusterSelector is fixed, Techsupport will be reconciled
			return requests
		}
		if parsedSelector.Matches(labels.Set(cluster.GetLabels())) {
			requests = append(requests, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Name: k.Name,
				},
			})
		}
	}

	return requests
}

// getClusterSummaryServiceAccountInfo returns the name of the ServiceAccount
// (presenting a tenant admin) that created the ClusterProfile instance owing this
// ClusterProfile instance
func getClusterSummaryServiceAccountInfo(techsupport *utilsv1alpha1.Techsupport) (namespace, name string) {
	if techsupport.Labels == nil {
		return "", ""
	}

	return techsupport.Labels[libsveltosv1alpha1.ServiceAccountNamespaceLabel],
		techsupport.Labels[libsveltosv1alpha1.ServiceAccountNameLabel]
}
