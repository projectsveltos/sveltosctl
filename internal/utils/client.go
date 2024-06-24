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

package utils

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	memory "k8s.io/client-go/discovery/cached"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	eventv1beta1 "github.com/projectsveltos/event-manager/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
)

// k8sAccess is used to access resources in the management cluster.
type k8sAccess struct {
	client     client.Client
	restConfig *rest.Config
	clientset  *kubernetes.Clientset
	scheme     *runtime.Scheme
}

var (
	accessInstance *k8sAccess
)

// GetAccessInstance return k8sAccess instance used to access resources in the
// management cluster.
func GetAccessInstance() *k8sAccess {
	return accessInstance
}

// Following method could have been called directly by GetAccessInstance is accessInstance was
// nil. Doing this way though it makes it possible to run uts against each of the implemented
// command.

// InitalizeManagementClusterAcces initializes k8sAccess singleton
func InitalizeManagementClusterAcces(scheme *runtime.Scheme, restConfig *rest.Config,
	cs *kubernetes.Clientset, c client.Client) {

	accessInstance = &k8sAccess{
		scheme:     scheme,
		client:     c,
		clientset:  cs,
		restConfig: restConfig,
	}
}

func GetScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	if err := addToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func addToScheme(scheme *runtime.Scheme) error {
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := configv1alpha1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := utilsv1beta1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := clusterv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := libsveltosv1beta1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := eventv1beta1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return err
	}
	return nil
}

// GetScheme returns scheme
func (a *k8sAccess) GetScheme() *runtime.Scheme {
	return a.scheme
}

// GetClient returns scheme
func (a *k8sAccess) GetClient() client.Client {
	return a.client
}

// GetConfig returns restConfig
func (a *k8sAccess) GetConfig() *rest.Config {
	return a.restConfig
}

// ListNamespaces gets all namespaces.
func (a *k8sAccess) ListNamespaces(ctx context.Context, logger logr.Logger) (*corev1.NamespaceList, error) {
	logger.V(logs.LogDebug).Info("Get all Namespaces")
	list := &corev1.NamespaceList{}
	err := a.client.List(ctx, list, &client.ListOptions{})
	return list, err
}

// GetDynamicResourceInterface returns a dynamic ResourceInterface for the policy's GroupVersionKind
func (a *k8sAccess) GetDynamicResourceInterface(policy *unstructured.Unstructured) (dynamic.ResourceInterface, error) {
	dynClient, err := dynamic.NewForConfig(a.restConfig)
	if err != nil {
		return nil, err
	}

	gvk := policy.GroupVersionKind()

	dc, err := discovery.NewDiscoveryClientForConfig(a.restConfig)
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return nil, err
	}
	var dr dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		// namespaced resources should specify the namespace
		dr = dynClient.Resource(mapping.Resource).Namespace(policy.GetNamespace())
	} else {
		// for cluster-wide resources
		dr = dynClient.Resource(mapping.Resource)
	}

	return dr, nil
}
