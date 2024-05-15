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
    "path/filepath"

    apierrors "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/tools/clientcmd"
    "k8s.io/client-go/util/homedir"

    libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
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

// configures the Kubernetes client to target the correct cluster based on namespace and clusterName.
// func ConfigureClient(ctx context.Context, namespace, clusterName string) (*kubernetes.Clientset, error) {
//     kubeconfigPath := filepath.Join(homedir.HomeDir(), ".kube", "config")
//     config, err := clientcmd.LoadFromFile(kubeconfigPath)
//     if err != nil {
//         return nil, fmt.Errorf("failed to load kubeconfig from %s: %v", kubeconfigPath, err)
//     }

//     // use it to change the context if clusterName is specified
//     if clusterName != "" {
//         contextName := fmt.Sprintf("%s-context", clusterName)
//         if _, exists := config.Contexts[contextName]; !exists {
//             return nil, fmt.Errorf("no context found for the specified cluster name: %s", clusterName)
//         }
//         config.CurrentContext = contextName
//     }

//     // build a rest.Config from the kubeconfig and the overridden current context.
//     restConfig, err := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{CurrentContext: config.CurrentContext}).ClientConfig()
//     if err != nil {
//         return nil, fmt.Errorf("failed to create Kubernetes REST client configuration: %v", err)
//     }

//     // create the Kubernetes client using the configured REST config
//     clientset, err := kubernetes.NewForConfig(restConfig)
//     if err != nil {
//         return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
//     }

//     return clientset, nil
// }

func collectLogLevelConfiguration(ctx context.Context, namespace string, clusterName string) ([]*componentConfiguration, string, string, error) {
    instance := utils.GetAccessInstance()

    dc, err := instance.GetDebuggingConfiguration(ctx, namespace, clusterName)
    if err != nil {
        return nil, namespace, clusterName, err
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

func updateLogLevelConfiguration(
    ctx context.Context,
    namespace string,
    clusterName string,
    spec []libsveltosv1alpha1.ComponentConfiguration,
) (string, string, error) {

    instance := utils.GetAccessInstance()
    dc, err := instance.GetDebuggingConfiguration(ctx, namespace, clusterName)
    if err != nil {
        if apierrors.IsNotFound(err) {
            dc = &libsveltosv1alpha1.DebuggingConfiguration{
                ObjectMeta: metav1.ObjectMeta{
                    Namespace: namespace,
                    Name:      clusterName,
                },
            }
        } else {
            return namespace, clusterName, err
        }
    }

    dc.Spec = libsveltosv1alpha1.DebuggingConfigurationSpec{
        Configuration: spec,
    }

    err = instance.UpdateDebuggingConfiguration(ctx, namespace, clusterName, dc)
    return err
}
