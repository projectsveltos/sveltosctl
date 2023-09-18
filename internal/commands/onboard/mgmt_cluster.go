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
	"encoding/base64"
	"flag"
	"fmt"
	"os"
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

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/libsveltos/lib/clusterproxy"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	defaultNamespace       = "mgmt"
	defaultName            = "mgmt"
	clusterRoleName        = "mgmt-role"
	clusterRoleBindingName = "mgmt-role-binding"
	saName                 = "sveltos"
	saNamespace            = "projectsveltos"
	// expirationInSecond is the token expiration time.
	saExpirationInSecond = 365 * 24 * 60 * time.Minute
)

func createNamespace(ctx context.Context, clusterNamespace string, logger logr.Logger) error {
	if clusterNamespace != defaultNamespace {
		return nil // only if management cluster needs to be registered in the defaultNamespace
		// namespace will be created
	}

	instance := utils.GetAccessInstance()

	currentNs := &corev1.Namespace{}
	err := instance.GetClient().Get(ctx, types.NamespacedName{Name: clusterNamespace}, currentNs)
	if err == nil {
		return nil
	}

	if apierrors.IsNotFound(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Create namespace %s", clusterNamespace))
		// If namespace defaultNamespace does not exist, create it
		currentNs = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: defaultNamespace,
			},
		}

		return instance.GetClient().Create(ctx, currentNs)
	}

	return err
}

func createSecretWithKubeconfig(ctx context.Context, clusterNamespace, clusterName, kubeconfigPath string,
	logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	secretName := clusterName + sveltosKubeconfigSecretNamePostfix
	logger.V(logs.LogDebug).Info(
		fmt.Sprintf("Verifying Secret %s/%s does not exist already", clusterNamespace, secretName))

	secret := &corev1.Secret{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Read file
	_, err = os.ReadFile(kubeconfigPath)
	if err != nil {
		return err
	}

	return createSecret(ctx, clusterNamespace, secretName, kubeconfigPath, logger)
}

// getServiceAccountToken returns token for a serviceaccount
func getServiceAccountToken(ctx context.Context,
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
		fmt.Sprintf("Create Token for ServiceAccount %s/%s", saNamespace, saName))
	var tokenRequest *authenticationv1.TokenRequest
	tokenRequest, err = clientset.CoreV1().ServiceAccounts(saNamespace).
		CreateToken(ctx, saName, treq, metav1.CreateOptions{})
	if err != nil {
		logger.V(logs.LogDebug).Info(
			fmt.Sprintf("Failed to create token for ServiceAccount %s/%s: %v",
				saNamespace, saName, err))
		return nil, err
	}

	return &tokenRequest.Status, nil
}

// getKubeconfigFromToken returns Kubeconfig to access management cluster from token.
func getKubeconfigFromToken(token string) string {
	template := `apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: %s
    certificate-authority-data: "%s"
users:
- name: sveltos
  user:
    token: %s
contexts:
- name: sveltos-context
  context:
    cluster: local
    user: %s
current-context: sveltos-context`

	instance := utils.GetAccessInstance()

	data := fmt.Sprintf(template, instance.GetConfig().Host,
		base64.StdEncoding.EncodeToString(instance.GetConfig().CAData), token, saName)

	return data
}

func createClusterRole(ctx context.Context, logger logr.Logger) error {
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

func createClusterRoleBinding(ctx context.Context, logger logr.Logger) error {
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
				Namespace: saNamespace,
				Name:      saName,
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

func createServiceAccount(ctx context.Context, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create ServiceAccount %s/%s", saNamespace, saName))
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: saNamespace,
			Name:      saName,
		},
	}

	// Create ServiceAccount
	err := instance.GetClient().Create(ctx, sa)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to created ServiceAccount %s/%s: %v",
			saNamespace, saName, err))
		return err
	}

	return nil
}

// grantSveltosClusterAdminRole grants Sveltos * permissions.
// Sveltos addon-controller serviceAccount is tied to addon-controller-role-extra
// ClusterRole.
// This command extends permission in such cluster. Then takes the kubeconfig associated
// with the Sveltos addon-controller serviceAccount and store in the Secret for the
// SveltosCluster corresponding to the management cluster.
func grantSveltosClusterAdminRole(ctx context.Context, clusterNamespace, clusterName string,
	logger logr.Logger) error {

	err := createClusterRole(ctx, logger)
	if err != nil {
		return err
	}

	err = createClusterRoleBinding(ctx, logger)
	if err != nil {
		return err
	}

	err = createServiceAccount(ctx, logger)
	if err != nil {
		return err
	}

	token, err := getServiceAccountToken(ctx, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("Get Kubeconfig from Token")
	data := getKubeconfigFromToken(token.Token)
	kubeconfig, err := clusterproxy.CreateKubeconfig(logger, []byte(data))
	if err != nil {
		return err
	}
	defer os.Remove(kubeconfig)

	logger.V(logs.LogDebug).Info("Create secret with Kubeconfig")
	err = createSecretWithKubeconfig(ctx, clusterNamespace, clusterName, kubeconfig, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("Create SveltosCluster")
	err = createSveltosCluster(ctx, clusterNamespace, clusterName, logger)
	if err != nil {
		return err
	}

	return nil
}

func onboardMgmtCluster(ctx context.Context, clusterNamespace, clusterName, kubeconfigPath string,
	logger logr.Logger) error {

	err := createNamespace(ctx, clusterNamespace, logger)
	if err != nil {
		return err
	}

	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Verifying SveltosCluster %s/%s does not exist already", clusterNamespace, clusterName))
	sveltosCluster := &libsveltosv1alpha1.SveltosCluster{}
	err = instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// if Kubeconfig is provided, use it
	if kubeconfigPath != "" {
		return createSecretWithKubeconfig(ctx, clusterNamespace, clusterName, kubeconfigPath, logger)
	}

	// No Kubeconfig provided. Sveltos will be granted cluster-admin role
	return grantSveltosClusterAdminRole(ctx, clusterNamespace, clusterName, logger)
}

// RegisterManagementCluster takes care of creating all necessary internal resources to import a cluster
func RegisterManagementCluster(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl register mgmt-cluster [options] [--namespace=<name>] [--cluster=<name>] [--kubeconfig=<file>] [--verbose]

     --namespace=<name>      The namespace where SveltosCluster will be created. By default "mgmt" will be used.
     --cluster=<name>        The name of the SveltosCluster. By default "mgmt" will be used.
     --kubeconfig=<file>     Path of the file containing the cluster kubeconfig. If not provided, Sveltos
                             will be given cluster-admin access to the management cluster.
                             If kubeconfig is not passed, run this command only while using sveltosctl as binary.
                             Sveltosctl as pod does not have enough permission to execute necessary code. 

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The register mgmt-cluster command registers the management cluster as a cluster to be managed by for Sveltos.
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

	namespace := defaultNamespace
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := defaultName
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	kubeconfig := ""
	if passedKubeconfig := parsedArgs["--kubeconfig"]; passedKubeconfig != nil {
		kubeconfig = passedKubeconfig.(string)
	}

	return onboardMgmtCluster(ctx, namespace, cluster, kubeconfig, logger)
}
