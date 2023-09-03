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

package onboard

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/commands/generate"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	//nolint: gosec // Sveltos secret postfix
	sveltosKubeconfigSecretNamePostfix = "-sveltos-kubeconfig"
	kubeconfig                         = "kubeconfig"
)

func onboardSveltosCluster(ctx context.Context, clusterNamespace, clusterName string, kubeconfigData []byte,
	labels map[string]string, renew bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	secretName := clusterName + sveltosKubeconfigSecretNamePostfix
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Verifying Secret %s/%s does not exist already", clusterNamespace, secretName))
	secret := &corev1.Secret{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = patchSveltosCluster(ctx, clusterNamespace, clusterName, labels, renew, logger)
	if err != nil {
		return err
	}

	err = patchSecret(ctx, clusterNamespace, secretName, kubeconfigData, logger)
	if err != nil {
		return err
	}

	//nolint: forbidigo // print success message
	fmt.Printf("cluster %s successfully registered/updated in namespace %s.", clusterName, clusterNamespace)
	return nil
}

func patchSveltosCluster(ctx context.Context, clusterNamespace, clusterName string,
	labels map[string]string, renew bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	currentSveltosCluster := &libsveltosv1alpha1.SveltosCluster{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName},
		currentSveltosCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating SveltosCluster %s/%s", clusterNamespace, clusterName))
			currentSveltosCluster.Namespace = clusterNamespace
			currentSveltosCluster.Name = clusterName
			currentSveltosCluster.Labels = labels
			if renew {
				currentSveltosCluster.Spec.TokenRequestRenewalOption = &libsveltosv1alpha1.TokenRequestRenewalOption{
					RenewTokenRequestInterval: metav1.Duration{Duration: 24 * time.Hour},
				}
			}

			return instance.CreateResource(ctx, currentSveltosCluster)
		}
		return err
	}

	logger.V(logs.LogDebug).Info("Updating SveltosCluster")
	currentSveltosCluster.Labels = labels
	return instance.UpdateResource(ctx, currentSveltosCluster)
}

func patchSecret(ctx context.Context, clusterNamespace, secretName string, kubeconfigData []byte, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentSecret := &corev1.Secret{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, currentSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating Secret %s/%s", clusterNamespace, secretName))
			currentSecret.Namespace = clusterNamespace
			currentSecret.Name = secretName
			currentSecret.Data = map[string][]byte{kubeconfig: kubeconfigData}
			return instance.CreateResource(ctx, currentSecret)
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Secret %s/%s", clusterNamespace, secretName))
	currentSecret.Data = map[string][]byte{
		kubeconfig: kubeconfigData,
	}

	return instance.UpdateResource(ctx, currentSecret)
}

// RegisterCluster takes care of creating all necessary internal resources to import a cluster
func RegisterCluster(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl register cluster [options] --namespace=<name> --cluster=<name> [--kubeconfig=<file>] [--fleet-cluster-context=<value>] [--labels=<value>] 
                              [--verbose]

     --namespace=<name>                  Specifies the namespace where Sveltos will create a resource (SveltosCluster) to represent the registered cluster.
     --cluster=<name>                    Defines a name for the registered cluster within Sveltos.
     --kubeconfig=<file>                 (Optional) Provides the path to a file containing the kubeconfig for the Kubernetes cluster you want to register.
                                         If you don't have a kubeconfig file yet, you can use the "sveltosctl generate kubeconfig" command. Be sure 
                                         to point that command to the specific cluster you want to manage. This will help you create the necessary 
                                         kubeconfig file before registering the cluster with Sveltos.
                                         Either --kubeconfig or --fleet-cluster-context must be provided.
     --fleet-cluster-context=<value>     (Optional) If your kubeconfig has multiple contexts:
                                         - One context points to the management cluster (default one)
                                         - Another context points to the cluster you actually want to manage;
                                         In this case, you can specify the context name with the --fleet-cluster-context flag. This tells
                                         the command to use the specific context to generate a Kubeconfig Sveltos can use and then create
                                         a SveltosCluster with it so you don't have to provide kubeconfig
                                         Either --kubeconfig or --fleet-cluster-context must be provided.
     --labels=<key1=value1,key2=value2>  (Optional) This option allows you to specify labels for the SveltosCluster resource being created.
                                         The format for labels is <key1=value1,key2=value2>, where each key-value pair is separated by a comma (,) and 
                                         the key and value are separated by an equal sign (=). You can define multiple labels by adding more key-value pairs
                                         separated by commas.


Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The register cluster command registers a cluster to be managed by Sveltos.
`
	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}
	if len(parsedArgs) == 0 {
		return nil
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug))
		if err != nil {
			return err
		}
	}

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	var labels map[string]string
	if passedLabels := parsedArgs["--labels"]; passedLabels != nil {
		labels, err = stringToMap(passedLabels.(string))
		if err != nil {
			return err
		}
	}

	kubeconfig := ""
	if passedKubeconfig := parsedArgs["--kubeconfig"]; passedKubeconfig != nil {
		kubeconfig = passedKubeconfig.(string)
	}

	renew := false
	fleetClusterContext := ""
	if passedContext := parsedArgs["--fleet-cluster-context"]; passedContext != nil {
		fleetClusterContext = passedContext.(string)
		renew = true
	}

	if kubeconfig == "" && fleetClusterContext == "" {
		return fmt.Errorf("either kubeconfig or fleet-cluster-context must be specified")
	}

	data, err := getKubeconfigData(ctx, kubeconfig, fleetClusterContext, logger)
	if err != nil {
		return err
	}

	return onboardSveltosCluster(ctx, namespace, cluster, data, labels, renew, logger)
}

func getKubeconfigData(ctx context.Context, kubeconfigFile, fleetClusterContext string, logger logr.Logger) ([]byte, error) {
	var data []byte
	if fleetClusterContext != "" {
		currentContext, err := getCurrentContext()
		if err != nil {
			return nil, err
		}
		kubeconfigData, err := createKubeconfig(ctx, fleetClusterContext, logger)
		if err != nil {
			return nil, err
		}
		err = switchCurrentContext(currentContext)
		if err != nil {
			return nil, err
		}
		data = []byte(kubeconfigData)
	} else {
		var err error
		data, err = os.ReadFile(kubeconfigFile)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

func stringToMap(data string) (map[string]string, error) {
	const keyValueLength = 2
	result := make(map[string]string)
	for _, pair := range strings.Split(data, ",") {
		kv := strings.Split(pair, "=")
		if len(kv) != keyValueLength {
			return nil, fmt.Errorf("invalid key-value pair format: %s", pair)
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		result[key] = value
	}
	return result, nil
}

func createKubeconfig(ctx context.Context, fleetClusterContext string, logger logr.Logger) (string, error) {
	logger.V(logs.LogDebug).Info("Get current context")
	currentContext, err := getCurrentContext()
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("current context %s", currentContext))

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Switch context to %s", fleetClusterContext))
	err = switchCurrentContext(fleetClusterContext)
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("switched to context %s", fleetClusterContext))

	var remoteRestConfig *rest.Config
	remoteRestConfig, err = ctrl.GetConfig()
	if err != nil {
		return "", err
	}

	logger.V(logs.LogDebug).Info("Generate Kubeconfig")
	var data string
	data, err = generate.GenerateKubeconfigForServiceAccount(ctx, remoteRestConfig, generate.Projectsveltos, generate.Projectsveltos,
		0, true, false, logger)
	if err != nil {
		return "", err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Reset context to %s", currentContext))
	err = switchCurrentContext(currentContext)
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("switched to context %s", currentContext))

	return data, nil
}

func getCurrentContext() (string, error) {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.RawConfig()
	if err != nil {
		return "", err
	}

	return config.CurrentContext, nil
}

func switchCurrentContext(fleetClusterContext string) error {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.RawConfig()

	if err != nil {
		return err
	}

	for contextName := range config.Contexts {
		if contextName == fleetClusterContext {
			config.CurrentContext = fleetClusterContext
			err = clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), config, true)
			if err != nil {
				return fmt.Errorf("error ModifyConfig: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("error context %s not found", fleetClusterContext)
}
