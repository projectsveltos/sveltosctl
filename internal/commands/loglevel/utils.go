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

package loglevel

import (
    "context"
    "sort"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

    libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
    "github.com/projectsveltos/sveltosctl/internal/utils"
)

type componentConfiguration struct {
    component   libsveltosv1alpha1.Component
    logSeverity libsveltosv1alpha1.LogLevel
}

// byComponent sorts componentConfiguration by name.
type byComponent []*componentConfiguration

func (c byComponent) Len() int      { return len(c) }
func (c byComponent) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byComponent) Less(i, j int) bool {
    return c[i].component < c[j].component
}

func collectLogLevelConfiguration(ctx context.Context, dc *libsveltosv1alpha1.DebuggingConfiguration) ([]*componentConfiguration, error) {
    configurationSettings := make([]*componentConfiguration, len(dc.Spec.Configuration))

    for i, c := range dc.Spec.Configuration {
        configurationSettings[i] = &componentConfiguration{
            component:   c.Component,
            logSeverity: c.LogLevel,
        }
    }

    // Sort this by component name first. Component/node is higher priority than Component
    sort.Sort(byComponent(configurationSettings))

    return configurationSettings, nil
}

func updateLogLevelConfiguration(
    ctx context.Context,
    spec []libsveltosv1alpha1.ComponentConfiguration,
    dc *libsveltosv1alpha1.DebuggingConfiguration,
) error {

    dc.Spec = libsveltosv1alpha1.DebuggingConfigurationSpec{
        Configuration: spec,
    }

    return instance.UpdateDebuggingConfiguration(ctx, dc)
}

// updates the log severity for a managed cluster
func updateDebuggingConfigurationInManaged(
    ctx context.Context,
    logSeverity libsveltosv1alpha1.LogLevel,
    component string,
    namespace string,
    clusterName string,
    clusterType string,
) error {
    
    // get a client for the managed cluster using; namespace, clusterName, and clusterType
    instance := utils.GetAccessInstance()
    client, err := instance.GetClientForManagedCluster(namespace, clusterName, clusterType)
    if err != nil {
        return err
    }

    // get the DebuggingConfiguration from the managed cluster
    dc, err := client.GetDebuggingConfiguration(ctx, namespace)
    if err != nil {
        if apierrors.IsNotFound(err) {
            dc = &libsveltosv1alpha1.DebuggingConfiguration{
                ObjectMeta: metav1.ObjectMeta{
                    Namespace: namespace,
                },
                Spec: libsveltosv1alpha1.DebuggingConfigurationSpec{},
            }
        } else {
            return err
        }
    }

    // update the log severity in the managed cluster
    return updateDebuggingConfiguration(ctx, logSeverity, component, dc)
}
