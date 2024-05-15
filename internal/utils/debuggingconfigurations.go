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
    "sigs.k8s.io/controller-runtime/pkg/client"
    "k8s.io/klog/v2"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
    clusterproxy "github.com/projectsveltos/libsveltos/lib/clusterproxy"
)

const (
	defaultInstanceName = "default"
)

// GetDebuggingConfiguration gets default DebuggingConfiguration in the specified namespace and cluster
func (a *k8sAccess) GetDebuggingConfiguration(
	ctx context.Context,
	clusterNamespace string, 
	clusterName string,
) (*libsveltosv1alpha1.DebuggingConfiguration, error) {

    req := &libsveltosv1alpha1.DebuggingConfiguration{}
    var c client.Client
    var err error
    logger := klog.FromContext(ctx)

	reqName := client.ObjectKey{
		Namespace: clusterNamespace,
		Name:      clusterName,
	}

    reqName := client.ObjectKey{
        Name:      defaultInstanceName,
        Namespace: namespace,
    }

    if err := c.Get(ctx, reqName, req); err != nil {
        return nil, err
    }

    return req, nil
}

// UpdateDebuggingConfiguration creates, if not existing already, default DebuggingConfiguration in the specified namespace and cluster. Otherwise
// updates it.
func (a *k8sAccess) UpdateDebuggingConfiguration(
	ctx context.Context,
	clusterNamespace, clusterName string,
	dc *libsveltosv1alpha1.DebuggingConfiguration,
) error {

	reqName := client.ObjectKey{
		Namespace: clusterNamespace,
		Name:      clusterName,
	}

    if namespace == "" && clusterName == "" && clusterType == "" {
        c = a.client
    } else {
        c, err = clusterproxy.GetKubernetesClient(ctx, a.client, namespace, clusterName, "", "", libsveltosv1alpha1.ClusterType(clusterType), logger)
        if err != nil {
            return err
        }
    }

    reqName := client.ObjectKey{
        Name:      defaultInstanceName,
        Namespace: namespace,
    }

    tmp := &libsveltosv1alpha1.DebuggingConfiguration{}

    err = c.Get(ctx, reqName, tmp)
    if err != nil {
        if apierrors.IsNotFound(err) {
            dc.Namespace = namespace
            err = c.Create(ctx, dc)
            if err != nil {
                return err
            }
        } else {
            return err
        }
    }

    dc.Namespace = namespace
    err = c.Update(ctx, dc)
    if err != nil {
        return err
    }

    return nil
}
