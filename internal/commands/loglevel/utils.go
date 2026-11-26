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
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	defaultDebuggingConfigurationName = "default"
)

// getManagedClusterClient returns a controller-runtime client for the managed
// cluster identified by (namespace, clusterName, clusterType), using the
// management cluster client to look up credentials via libsveltos clusterproxy.
func getManagedClusterClient(ctx context.Context, namespace, clusterName string,
	clusterType libsveltosv1beta1.ClusterType) (client.Client, error) {

	managedClient, err := clusterproxy.GetKubernetesClient(ctx, utils.GetAccessInstance().GetClient(),
		namespace, clusterName, "", "", clusterType, logr.Discard())
	if err != nil {
		return nil, fmt.Errorf("failed to get client for managed cluster %s/%s: %w",
			namespace, clusterName, err)
	}
	return managedClient, nil
}

// parseManagedClusterArgs extracts the optional --namespace, --clusterName and
// --clusterType values from the parsed docopt args. clusterType defaults to
// Capi; any value other than "Sveltos" (case-sensitive) falls back to Capi for
// backward compatibility with existing invocations.
func parseManagedClusterArgs(parsedArgs map[string]interface{}) (
	namespace, clusterName string, clusterType libsveltosv1beta1.ClusterType, err error) {

	if v := parsedArgs["--namespace"]; v != nil {
		namespace = v.(string)
	}
	if v := parsedArgs["--clusterName"]; v != nil {
		clusterName = v.(string)
	}

	clusterType = libsveltosv1beta1.ClusterTypeCapi
	if v := parsedArgs["--clusterType"]; v != nil {
		switch v.(string) {
		case string(libsveltosv1beta1.ClusterTypeSveltos):
			clusterType = libsveltosv1beta1.ClusterTypeSveltos
		case string(libsveltosv1beta1.ClusterTypeCapi):
			clusterType = libsveltosv1beta1.ClusterTypeCapi
		default:
			return "", "", "", fmt.Errorf(
				"invalid --clusterType %q: must be %q or %q",
				v, libsveltosv1beta1.ClusterTypeCapi, libsveltosv1beta1.ClusterTypeSveltos)
		}
	}

	return namespace, clusterName, clusterType, nil
}

type componentConfiguration struct {
	component   libsveltosv1beta1.Component
	logSeverity libsveltosv1beta1.LogLevel
}

// byComponent sorts componentConfiguration by name.
type byComponent []*componentConfiguration

func (c byComponent) Len() int      { return len(c) }
func (c byComponent) Swap(i, j int) { c[i], c[j] = c[j], c[i] }
func (c byComponent) Less(i, j int) bool {
	return c[i].component < c[j].component
}

func collectLogLevelConfiguration(ctx context.Context) ([]*componentConfiguration, error) {
	instance := utils.GetAccessInstance()

	dc, err := instance.GetDebuggingConfiguration(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return make([]*componentConfiguration, 0), nil
		}
		return nil, err
	}

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
	spec []libsveltosv1beta1.ComponentConfiguration,
) error {

	instance := utils.GetAccessInstance()

	dc, err := instance.GetDebuggingConfiguration(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			dc = &libsveltosv1beta1.DebuggingConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			}
		} else {
			return err
		}
	}

	dc.Spec = libsveltosv1beta1.DebuggingConfigurationSpec{
		Configuration: spec,
	}

	return instance.UpdateDebuggingConfiguration(ctx, dc)
}

// collectLogLevelConfigurationFromClient returns the current log-level
// configuration read from the DebuggingConfiguration "default" instance
// accessible through the provided client.
func collectLogLevelConfigurationFromClient(ctx context.Context,
	c client.Client) ([]*componentConfiguration, error) {

	dc := &libsveltosv1beta1.DebuggingConfiguration{}
	err := c.Get(ctx, client.ObjectKey{Name: defaultDebuggingConfigurationName}, dc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return make([]*componentConfiguration, 0), nil
		}
		return nil, err
	}

	configurationSettings := make([]*componentConfiguration, len(dc.Spec.Configuration))
	for i, c := range dc.Spec.Configuration {
		configurationSettings[i] = &componentConfiguration{
			component:   c.Component,
			logSeverity: c.LogLevel,
		}
	}

	sort.Sort(byComponent(configurationSettings))
	return configurationSettings, nil
}

// updateLogLevelConfigurationWithClient writes spec to the DebuggingConfiguration
// "default" instance accessible through the provided client, creating it if it
// does not exist.
func updateLogLevelConfigurationWithClient(ctx context.Context, c client.Client,
	spec []libsveltosv1beta1.ComponentConfiguration) error {

	dc := &libsveltosv1beta1.DebuggingConfiguration{}
	err := c.Get(ctx, client.ObjectKey{Name: defaultDebuggingConfigurationName}, dc)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
		dc = &libsveltosv1beta1.DebuggingConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultDebuggingConfigurationName,
			},
			Spec: libsveltosv1beta1.DebuggingConfigurationSpec{
				Configuration: spec,
			},
		}
		return c.Create(ctx, dc)
	}

	dc.Spec = libsveltosv1beta1.DebuggingConfigurationSpec{
		Configuration: spec,
	}
	return c.Update(ctx, dc)
}
