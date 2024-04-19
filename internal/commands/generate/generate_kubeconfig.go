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
	"strconv"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	Projectsveltos = "projectsveltos"
)

func GenerateKubeconfigForServiceAccount(ctx context.Context, remoteRestConfig *rest.Config, namespace, serviceAccountName string,
	expirationSeconds int, create, display bool, logger logr.Logger) (string, error) {

	s := runtime.NewScheme()
	err := clientgoscheme.AddToScheme(s)
	if err != nil {
		return "", err
	}

	var remoteClient client.Client
	remoteClient, err = client.New(remoteRestConfig, client.Options{Scheme: s})
	if err != nil {
		return "", err
	}

	if create {
		err = createNamespace(ctx, remoteClient, namespace, logger)
		if err != nil {
			return "", err
		}
		err = createServiceAccount(ctx, remoteClient, namespace, serviceAccountName, logger)
		if err != nil {
			return "", err
		}
		err = createClusterRole(ctx, remoteClient, Projectsveltos, logger)
		if err != nil {
			return "", err
		}
		err = createClusterRoleBinding(ctx, remoteClient, Projectsveltos, Projectsveltos, namespace,
			serviceAccountName, logger)
		if err != nil {
			return "", err
		}
	} else {
		err = getNamespace(ctx, remoteClient, namespace)
		if err != nil {
			return "", err
		}
		err = getServiceAccount(ctx, remoteClient, namespace, serviceAccountName)
		if err != nil {
			return "", err
		}
	}

	tokenRequest, err := getServiceAccountTokenRequest(ctx, remoteRestConfig, namespace, serviceAccountName,
		expirationSeconds, logger)
	if err != nil {
		return "", err
	}

	logger.V(logs.LogDebug).Info("Get Kubeconfig from TokenRequest")
	data := getKubeconfigFromToken(remoteRestConfig, namespace, serviceAccountName, tokenRequest.Token)
	if display {
		//nolint: forbidigo // print kubeconfig
		fmt.Println(data)
	}

	return data, nil
}

func getNamespace(ctx context.Context, remoteClient client.Client, name string) error {
	currentNs := &corev1.Namespace{}
	return remoteClient.Get(ctx, types.NamespacedName{Name: name}, currentNs)
}

func createNamespace(ctx context.Context, remoteClient client.Client, name string, logger logr.Logger) error {
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create namespace %s", name))
	currentNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := remoteClient.Create(ctx, currentNs)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create Namespace %s: %v",
			name, err))
		return err
	}

	return nil
}

func getServiceAccount(ctx context.Context, remoteClient client.Client, namespace, name string) error {
	currentSA := &corev1.ServiceAccount{}
	return remoteClient.Get(ctx,
		types.NamespacedName{Namespace: namespace, Name: name},
		currentSA)
}

func createServiceAccount(ctx context.Context, remoteClient client.Client, namespace, name string,
	logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create serviceAccount %s/%s", namespace, name))
	currentSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
	}

	err := remoteClient.Create(ctx, currentSA)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create ServiceAccount %s/%s: %v",
			namespace, name, err))
		return err
	}

	return nil
}

func createClusterRole(ctx context.Context, remoteClient client.Client, clusterRoleName string,
	logger logr.Logger) error {

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

	err := remoteClient.Create(ctx, clusterrole)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create ClusterRole %s: %v",
			clusterRoleName, err))
		return err
	}

	return nil
}

func createClusterRoleBinding(ctx context.Context, remoteClient client.Client,
	clusterRoleName, clusterRoleBindingName, serviceAccountNamespace, serviceAccountName string,
	logger logr.Logger) error {

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
	err := remoteClient.Create(ctx, clusterrolebinding)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Failed to create clusterrolebinding %s: %v",
			clusterRoleBindingName, err))
		return err
	}

	return nil
}

// getServiceAccountTokenRequest returns token for a serviceaccount
func getServiceAccountTokenRequest(ctx context.Context, remoteRestConfig *rest.Config, serviceAccountNamespace, serviceAccountName string,
	expirationSeconds int, logger logr.Logger) (*authenticationv1.TokenRequestStatus, error) {

	expiration := int64(expirationSeconds)

	treq := &authenticationv1.TokenRequest{}

	if expirationSeconds != 0 {
		treq.Spec = authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expiration,
		}
	}

	clientset, err := kubernetes.NewForConfig(remoteRestConfig)
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
func getKubeconfigFromToken(remoteRestConfig *rest.Config, namespace, serviceAccountName, token string) string {
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

	data := fmt.Sprintf(template, remoteRestConfig.Host,
		base64.StdEncoding.EncodeToString(remoteRestConfig.CAData), serviceAccountName, token, namespace, serviceAccountName)

	return data
}

// GenerateKubeconfig creates a TokenRequest and a Kubeconfig associated with it
func GenerateKubeconfig(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl generate kubeconfig [options] [--namespace=<name>] [--serviceaccount=<name>] [--create] [--expirationSeconds=<value>] [--verbose]

     --namespace=<name>           (Optional) Specifies the namespace of the ServiceAccount to use. If not provided,
                                  the "projectsveltos" namespace will be used.
     --serviceaccount=<name>      (Optional) Specifies the name of the ServiceAccount to use. If not provided,
                                  "projectsveltos" will be used.
     --create                     (Optional) If set, Sveltos will create the necessary resources if they don't already exist:
                                  - The specified namespace (if not already present)
                                  - The specified ServiceAccount (if not already present)
                                  - A ClusterRole with cluster-admin permissions
                                  - A ClusterRoleBinding granting the ServiceAccount cluster-admin permissions
     --expirationSeconds=<value>  - (Optional) This option allows you to specify the desired validity period 
                                  (in seconds) for the token requested when generating a kubeconfig. Minimum value is 600 (10 minutes).
                                  If you don't provide this option, the issuer (where the kubeconfig points)
                                  will use its default expiration time for the token.
                                  Once you register a cluster using the kubeconfig generated by this command,
                                  you can manage automatic token renewal through the
                                  SveltosCluster.Spec.TokenRequestRenewalOption setting within the registered
                                  SveltosCluster resource. This provides more control over token expiration and renewal behavior. 

Process:

Sveltos will either use an existing ServiceAccount with sufficient permissions (if --create is not set) or create a new one with 
cluster-admin permissions (if --create is set).
Sveltos will generate a TokenRequest for the chosen ServiceAccount. Based on the TokenRequest, Sveltos will generate a kubeconfig 
file and output it.
The Kubeconfig can then be used with "sveltosctl register cluster" command.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
This command helps you set up credentials (kubeconfig) to access a Kubernetes cluster using Sveltos. It allows you to specify a ServiceAccount 
or create a new one with the necessary permissions.
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

	namespace := Projectsveltos
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	serviceAccount := Projectsveltos
	if passedServiceAccount := parsedArgs["--serviceaccount"]; passedServiceAccount != nil {
		serviceAccount = passedServiceAccount.(string)
	}

	expirationSeconds := 0
	if passedExpirationSeconds := parsedArgs["--expirationSeconds"]; passedExpirationSeconds != nil {
		expirationSeconds, err = strconv.Atoi(passedExpirationSeconds.(string))
		if err != nil {
			return err
		}
	}

	create := parsedArgs["--create"].(bool)

	_, err = GenerateKubeconfigForServiceAccount(ctx, utils.GetAccessInstance().GetConfig(),
		namespace, serviceAccount, expirationSeconds, create, true, logger)
	return err
}
