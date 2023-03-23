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
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// retryableErrorTimeout is the time process sleep before a retryble error is hit
	retryableErrorTimeout = time.Second
)

// ListResources retrieves list of objects for a given namespace and list options.
// It is a simple wrapper around List, retrying in case of retryable error.
func (a *k8sAccess) ListResources(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	i := 0
	for {
		err := a.client.List(ctx, list, opts...)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// UpdateResource creates or updates a resource in a CAPI Cluster.
func (a *k8sAccess) UpdateResourceWithDynamicResourceInterface(ctx context.Context, dr dynamic.ResourceInterface,
	object *unstructured.Unstructured, logger logr.Logger) error {

	l := logger.WithValues("resourceNamespace", object.GetNamespace(),
		"resourceName", object.GetName(), "resourceGVK", object.GetObjectKind().GroupVersionKind())
	l.V(logs.LogDebug).Info("deploying policy")

	data, err := runtime.Encode(unstructured.UnstructuredJSONScheme, object)
	if err != nil {
		return err
	}

	forceConflict := true
	options := metav1.PatchOptions{
		FieldManager: "application/apply-patch",
		Force:        &forceConflict,
	}
	_, err = dr.Patch(ctx, object.GetName(), types.ApplyPatchType, data, options)
	return err
}

// CreateResource saves the object obj in the Kubernetes cluster.
// It is a simple wrapper around Create, retrying in case of retryable error.
func (a *k8sAccess) CreateResource(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	i := 0
	for {
		err := a.client.Create(ctx, obj, opts...)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// GetResource retrieves an obj for the given object key from the Kubernetes Cluster.
// obj must be a struct pointer so that obj can be updated with the response returned by the Server.
// It is a simple wrapper around Get, retrying in case of retryable error.
func (a *k8sAccess) GetResource(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	i := 0
	for {
		err := a.client.Get(ctx, key, obj)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// UpdateResource updates an obj.
// It is a simple wrapper around Update, retrying in case of retryable error.
func (a *k8sAccess) UpdateResource(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	i := 0
	for {
		err := a.client.Update(ctx, obj, opts...)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// UpdateResourceStatus updates an obj Status
// It is a simple wrapper around Status().Update, retrying in case of retryable error.
func (a *k8sAccess) UpdateResourceStatus(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	i := 0
	for {
		err := a.client.Status().Update(ctx, obj, opts...)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// DeleteResource deletes deletes the given obj from Kubernetes cluster.
// It is a simple wrapper around Delete, retrying in case of retryable error.
func (a *k8sAccess) DeleteResource(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	i := 0
	for {
		err := a.client.Delete(ctx, obj, opts...)
		if err != nil {
			if shouldRetry(err, i) {
				time.Sleep(retryableErrorTimeout)
				i++
			} else {
				return err
			}
		} else {
			break
		}
	}

	return nil
}

// shouldRetry returns true when error is a retriable one
func shouldRetry(err error, i int) bool {
	const maxRetry = 10
	if i >= maxRetry {
		return false
	}
	if apierrors.IsInternalError(err) || apierrors.IsTimeout(err) || apierrors.IsTooManyRequests(err) {
		return true
	}
	// if the error sends the Retry-After header, we respect it as an explicit confirmation we should retry.
	if _, shouldRetry := apierrors.SuggestsClientDelay(err); shouldRetry {
		return true
	}
	return false
}
