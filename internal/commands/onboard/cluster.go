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

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	//nolint: gosec // Sveltos secret postfix
	sveltosKubeconfigSecretNamePostfix = "-sveltos-kubeconfig"
)

func onboardSveltosCluster(ctx context.Context, clusterNamespace, clusterName, kubeconfigPath string, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Verifying SveltosCluster %s/%s does not exist already", clusterNamespace, clusterName))
	sveltosCluster := &libsveltosv1alpha1.SveltosCluster{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	secretName := clusterName + sveltosKubeconfigSecretNamePostfix
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Verifying Secret %s/%s does not exist already", clusterNamespace, secretName))
	secret := &corev1.Secret{}
	err = instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Read file
	_, err = os.ReadFile(kubeconfigPath)
	if err != nil {
		return err
	}

	err = createSveltosCluster(ctx, clusterNamespace, clusterName, logger)
	if err != nil {
		return err
	}

	return createSecret(ctx, clusterNamespace, secretName, kubeconfigPath, logger)
}

func createSveltosCluster(ctx context.Context, clusterNamespace, clusterName string, logger logr.Logger) error {
	instance := utils.GetAccessInstance()
	logger.V(logs.LogDebug).Info("Create SveltosCluster")
	sveltosCluster := &libsveltosv1alpha1.SveltosCluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterNamespace,
			Name:      clusterName,
		},
	}
	err := instance.CreateResource(ctx, sveltosCluster)
	if err != nil && apierrors.IsAlreadyExists(err) {
		return nil
	}

	return err
}

func createSecret(ctx context.Context, clusterNamespace, secretName, kubeconfigPath string, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	var data []byte
	data, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("Create Secret")
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: clusterNamespace,
			Name:      secretName,
		},
		Data: map[string][]byte{
			"value": data,
		},
	}
	return instance.CreateResource(ctx, secret)
}

// RegisterCluster takes care of creating all necessary internal resources to import a cluster
func RegisterCluster(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl register cluster [options] --namespace=<name> --cluster=<name> --kubeconfig=<file> [--verbose]

     --namespace=<name>      The namespace where SveltosCluster will be created.
     --cluster=<name>        The name of the SveltosCluster.
     --kubeconfig=<file>     Path of the file containing the cluster kubeconfig.

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

	kubeconfig := ""
	if passedKubeconfig := parsedArgs["--kubeconfig"]; passedKubeconfig != nil {
		kubeconfig = passedKubeconfig.(string)
	}

	return onboardSveltosCluster(ctx, namespace, cluster, kubeconfig, logger)
}
