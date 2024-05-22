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
    "github.com/go-logr/logr"
    apierrors "k8s.io/apimachinery/pkg/api/errors"
    libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
    clusterproxy "github.com/projectsveltos/libsveltos/lib/clusterproxy"
)

// GetDebuggingConfiguration gets default DebuggingConfiguration in the specified namespace and cluster
func (a *k8sAccess) GetDebuggingConfiguration(
    ctx context.Context,
    namespace string,
    clusterName string,
    clusterType string,
    logger logr.Logger,
) (*libsveltosv1alpha1.DebuggingConfiguration, error) {

    req := &libsveltosv1alpha1.DebuggingConfiguration{}
    var c client.Client
    var err error

    if namespace == "" && clusterName == "" && clusterType == "" {
        c = a.client
    } else {
        c, err = clusterproxy.GetSveltosKubernetesClient(ctx, logger, a.client, a.scheme, namespace, clusterName)
        if err != nil {
            return nil, err
        }
    }

    reqName := client.ObjectKey{
        Namespace: namespace,
        Name:      clusterName,
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
    dc *libsveltosv1alpha1.DebuggingConfiguration,
    namespace string,
    clusterName string,
    clusterType string,
    logger logr.Logger,
) error {

    var c client.Client
    var err error

    if namespace == "" && clusterName == "" && clusterType == "" {
        c = a.client
    } else {
        c, err = clusterproxy.GetSveltosKubernetesClient(ctx, logger, a.client, a.scheme, namespace, clusterName)
        if err != nil {
            return err
        }
    }

    reqName := client.ObjectKey{
        Namespace: namespace,
        Name:      clusterName,
    }

    tmp := &libsveltosv1alpha1.DebuggingConfiguration{}

    err = c.Get(ctx, reqName, tmp)
    if err != nil {
        if apierrors.IsNotFound(err) {
            dc.Namespace = namespace
            dc.Name = clusterName
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
