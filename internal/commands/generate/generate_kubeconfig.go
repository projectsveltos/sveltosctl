/*
Copyright 2024. projectsveltos.io. All rights reserved.

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

package generate

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	projectsveltos = "projectsveltos"
	// expirationInSecond is the token expiration time.
	saExpirationInSecond = 365 * 24 * 60 * time.Minute
)

func generateKubeconfigForServiceAccount(ctx context.Context, namespace, serviceAccountName string,
	create bool, logger logr.Logger) error {

	if create {
		if err := createNamespace(ctx, namespace, logger); err != nil {
			return err
		}
		if err := createServiceAccount(ctx, namespace, serviceAccountName, logger); err != nil {
			return err
		}
		if err := createClusterRole(ctx, projectsveltos, logger); err != nil {
			return err
		}
		if err := createClusterRoleBinding(ctx, projectsveltos, projectsveltos, namespace, serviceAccountName, logger); err != nil {
			return err
		}
	} else {
		if err := getNamespace(ctx, namespace); err != nil {
			return err
		}
		if err := getServiceAccount(ctx, namespace, serviceAccountName); err != nil {
			return err
		}
	}

	tokenRequest, err := getServiceAccountTokenRequest(ctx, namespace, serviceAccountName, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("Get Kubeconfig from TokenRequest")
	data := getKubeconfigFromToken(namespace, serviceAccountName, tokenRequest.Token)
	//nolint: forbidigo // print kubeconfig
	fmt.Println(data)

	return nil
}

func getNamespace(ctx context.Context, name string) error {
	instance := utils.GetAccessInstance()
	currentNs := &corev1.Namespace{}
	return instance.GetClient().Get(ctx, types.NamespacedName{Name: name}, currentNs)
}

func createNamespace(ctx context.Context, name string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create namespace %s", name))
	currentNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	instance := utils.GetAccessInstance()
	err := instance.GetClient().Create(ctx, currentNs)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create Namespace %s: %v",
			name, err))
		return err
	}

	return nil
}

func getServiceAccount(ctx context.Context, namespace, name string) error {
	instance := utils.GetAccessInstance()
	currentSA := &corev1.ServiceAccount{}
	return instance.GetClient().Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: name},
		currentSA)
}

func createServiceAccount(ctx context.Context, namespace, name string,
	logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create serviceAccount %s/%s", namespace, name))
	currentSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}
	instance := utils.GetAccessInstance()
	err := instance.GetClient().Create(ctx, currentSA)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create ServiceAccount %s/%s: %v",
			namespace, name, err))
		return err
	}

	return nil
}

func createClusterRole(ctx context.Context, clusterRoleName string, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create ClusterRole %s", clusterRoleName))
	// Extends permission in addon-controller-role-extra
	clusterrole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"*"},
				APIGroups: []string{"*"},
				Resources: []string{"*"},
			},
			{
				Verbs:           []string{"*"},
				NonResourceURLs: []string{"*"},
			},
		},
	}

	err := instance.GetClient().Create(ctx, clusterrole)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create ClusterRole %s: %v",
			clusterRoleName, err))
		return err
	}

	return nil
}

func createClusterRoleBinding(ctx context.Context, clusterRoleName, clusterRoleBindingName, serviceAccountNamespace, serviceAccountName string,
	logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create ClusterRoleBinding %s", clusterRoleBindingName))
	clusterrolebinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleBindingName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Namespace: serviceAccountNamespace,
				Name:      serviceAccountName,
				Kind:      "ServiceAccount",
				APIGroup:  corev1.SchemeGroupVersion.Group,
			},
		},
	}
	err := instance.GetClient().Create(ctx, clusterrolebinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create clusterrolebinding %s: %v",
			clusterRoleBindingName, err))
		return err
	}

	return nil
}

// getServiceAccountTokenRequest returns token for a serviceaccount
func getServiceAccountTokenRequest(ctx context.Context, serviceAccountNamespace, serviceAccountName string,
	logger logr.Logger) (*authenticationv1.TokenRequestStatus, error) {

	instance := utils.GetAccessInstance()

	expiration := int64(saExpirationInSecond.Seconds())

	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiration,
		},
	}

	clientset, err := kubernetes.NewForConfig(instance.GetConfig())
	if err != nil {
		return nil, err
	}

	logger.V(logs.LogDebug).Info(
		fmt.Sprintf("Create Token for ServiceAccount %s/%s", serviceAccountNamespace, serviceAccountName))
	var tokenRequest *authenticationv1.TokenRequest
	tokenRequest, err = clientset.CoreV1().ServiceAccounts(serviceAccountNamespace).
		CreateToken(ctx, serviceAccountName, treq, metav1.CreateOptions{})
	if err != nil {
		logger.V(logs.LogDebug).Info(
			fmt.Sprintf("Failed to create token for ServiceAccount %s/%s: %v",
				serviceAccountNamespace, serviceAccountName, err))
		return nil, err
	}

	return &tokenRequest.Status, nil
}

// getKubeconfigFromToken returns Kubeconfig to access management cluster from token.
func getKubeconfigFromToken(namespace, serviceAccountName, token string) string {
	template := `apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: %s
    certificate-authority-data: "%s"
users:
- name: %s
  user:
    token: %s
contexts:
- name: sveltos-context
  context:
    cluster: local
    namespace: %s
    user: %s
current-context: sveltos-context`

	instance := utils.GetAccessInstance()

	data := fmt.Sprintf(template, instance.GetConfig().Host,
		base64.StdEncoding.EncodeToString(instance.GetConfig().CAData), serviceAccountName, token, namespace, serviceAccountName)

	return data
}

// GenerateKubeconfig creates a TokenRequest and a Kubeconfig associated with it
func GenerateKubeconfig(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl generate kubeconfig [options] [--namespace=<name>] [--serviceaccount=<name>] [--create] [--verbose]

     --namespace=<name>      The namespace of the ServiceAccount. If not specified, projectsveltos namespace will be used.
     --serviceaccount=<name> The name of the ServiceAccount. If not specified, projectsveltos will be used.
     --create                If a ServiceAccount with enough permissions is already present, do not set this flag.
                             Sveltos will generate a Kubeconfig associated to that ServiceAccount.
                             If a ServiceAccount with cluster admin permissions needs to be created, use this option.
                             When this option is set, this command will create necessary resources:
                             1. namespace if not existing already
                             2. serviceAccount if not existing already
                             3. ClusterRole with cluster admin permission
                             4. ClusterRoleBinding granting the serviceAccount cluster admin permissions
                             5. TokenRequest for the ServiceAccount							 

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The generate kubeconfig command will generate a Kubeconfig that can later on be used to register the cluster.
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

	namespace := projectsveltos
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	serviceAccount := projectsveltos
	if passedServiceAccount := parsedArgs["--serviceaccount"]; passedServiceAccount != nil {
		serviceAccount = passedServiceAccount.(string)
	}

	create := parsedArgs["--create"].(bool)

	return generateKubeconfigForServiceAccount(ctx, namespace, serviceAccount, create, logger)
}
