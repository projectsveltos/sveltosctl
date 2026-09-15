/*
Copyright 2025. projectsveltos.io. All rights reserved.

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
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/deployer"
	"github.com/projectsveltos/libsveltos/lib/k8s_utils"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/agent"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	// sveltosClusterManagerServiceAccount is the identity sveltoscluster-manager runs as in the
	// management cluster (fixed by the Helm chart). When --token is used, the per-cluster
	// Role/RoleBinding created below grants this identity permission to renew the pull-mode
	// cluster's token. The namespace it lives in is not fixed (Sveltos can be installed in any
	// namespace), so callers pass it in as sveltosNamespace, defaulting to "projectsveltos".
	sveltosClusterManagerServiceAccount = "sc-manager"

	tokenRenewalRBACNamePostfix = "-token-renewal"

	pullModeTokenDuration        = 24 * time.Hour
	pullModeTokenRenewalInterval = time.Hour

	// managementClusterURLConfigMapKey is the key, in the ConfigMap created by
	// createManagementClusterURLConfigMap, holding the management cluster's externally
	// reachable API server address.
	managementClusterURLConfigMapKey = "server"

	// managementClusterCAConfigMapKey is the key, in the same ConfigMap, holding the CA data
	// (PEM) that validates that address. Not necessarily the same CA as sveltoscluster-manager's
	// own in-cluster one: on some providers (observed on Civo) the externally reachable endpoint
	// is fronted by a load balancer presenting a certificate from a different CA.
	managementClusterCAConfigMapKey = "ca.crt"
)

func onboardSveltosClusterInPullMode(ctx context.Context, clusterNamespace, clusterName, shard, sveltosNamespace,
	managementClusterURL string, labels map[string]string, tokenRenewal bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()
	c := instance.GetClient()
	config := instance.GetConfig()

	err := createNamespace(ctx, c, clusterNamespace)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createNamespace failed: %s", err))
		return err
	}

	err = createServiceAccount(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createNamespace failed: %s", err))
		return err
	}

	if err := setupPullModeCredentials(ctx, c, clusterNamespace, clusterName, sveltosNamespace, managementClusterURL,
		tokenRenewal, config.CAData, logger); err != nil {
		return err
	}

	if err := createApplierRBAC(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		return err
	}

	err = createSveltosCluster(ctx, c, clusterNamespace, clusterName, shard, labels, tokenRenewal)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createSveltosCluster failed: %s", err))
		return err
	}

	var kubeconfig string
	if tokenRenewal {
		// Use the caller-provided externally reachable URL here too, not config.Host, so the
		// very first kubeconfig and every renewed one after it point at the same address.
		kubeconfig, err = getKubeconfigFromTokenRequest(ctx, config, managementClusterURL, clusterNamespace, clusterName, logger)
	} else {
		kubeconfig, err = getKubeconfig(ctx, c, clusterNamespace, clusterName, config.Host)
	}
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("getKubeconfig failed: %s", err))
		return err
	}

	toApplyYAML, err := prepareApplierYAML(kubeconfig, clusterNamespace, clusterName, logger)
	if err != nil {
		return err
	}

	//nolint: forbidigo // this is printing the YAML to apply to managed cluster
	fmt.Printf("%s", toApplyYAML)

	return nil
}

// setupPullModeCredentials creates whichever credential the managed cluster's kubeconfig is
// built from: the long-lived SA-token Secret (default), or, with --token, the per-cluster
// token-renewal RBAC plus the ConfigMap sveltoscluster-manager reads back on every renewal.
func setupPullModeCredentials(ctx context.Context, c client.Client, clusterNamespace, clusterName, sveltosNamespace,
	managementClusterURL string, tokenRenewal bool, caData []byte, logger logr.Logger) error {

	if !tokenRenewal {
		if err := createSecret(ctx, c, clusterNamespace, clusterName); err != nil {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("createSecret failed: %s", err))
			return err
		}
		return nil
	}

	if err := createTokenRenewalRole(ctx, c, clusterNamespace, clusterName); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createTokenRenewalRole failed: %s", err))
		return err
	}

	if err := createTokenRenewalRoleBinding(ctx, c, clusterNamespace, clusterName, sveltosNamespace); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createTokenRenewalRoleBinding failed: %s", err))
		return err
	}

	// sveltoscluster-manager runs in-cluster: its own rest.Config resolves to a cluster-internal
	// address (e.g. the kubernetes.default.svc ClusterIP), which is not reachable from the
	// managed cluster, and its in-cluster CA does not necessarily validate the externally
	// reachable endpoint either (observed on Civo: the external load balancer presents a
	// certificate from a different CA than the in-cluster one). Persist both the externally
	// reachable address and the CA that already correctly validates it (this command's own
	// ambient config.CAData, proven to work since it is what the initial kubeconfig is also
	// built from) so every future renewal embeds the right server and CA in the kubeconfig it
	// delivers to sveltos-applier.
	if err := createManagementClusterURLConfigMap(ctx, c, clusterNamespace, clusterName, managementClusterURL,
		caData); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createManagementClusterURLConfigMap failed: %s", err))
		return err
	}

	return nil
}

// createApplierRBAC creates the Role/ClusterRole/RoleBinding/ClusterRoleBinding sveltos-applier's
// own ServiceAccount needs, common to both --token and the default (non-renewing) registration.
func createApplierRBAC(ctx context.Context, c client.Client, clusterNamespace, clusterName string,
	logger logr.Logger) error {

	if err := createRole(ctx, c, clusterNamespace, clusterName); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRole failed: %s", err))
		return err
	}

	if err := createClusterRole(ctx, c, clusterNamespace, clusterName); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRole failed: %s", err))
		return err
	}

	if err := createRoleBinding(ctx, c, clusterNamespace, clusterName); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRoleBinding failed: %s", err))
		return err
	}

	if err := createClusterRoleBinding(ctx, c, clusterNamespace, clusterName); err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createClusterRoleBinding failed: %s", err))
		return err
	}

	return nil
}

func modifyDeployment(depl *appsv1.Deployment, clusterNamespace, clusterName string,
	logger logr.Logger) (*appsv1.Deployment, error) {

	clusterType := "sveltos"
	found := false
	for i := range depl.Spec.Template.Spec.Containers {
		container := &depl.Spec.Template.Spec.Containers[i]
		if container.Name != "controller" {
			continue
		}
		newArgs := []string{}
		for _, arg := range container.Args {
			if strings.HasPrefix(arg, "--cluster-namespace=") {
				newArgs = append(newArgs, fmt.Sprintf("--cluster-namespace=%s", clusterNamespace))
			} else if strings.HasPrefix(arg, "--cluster-name=") {
				newArgs = append(newArgs, fmt.Sprintf("--cluster-name=%s", clusterName))
			} else if strings.HasPrefix(arg, "--cluster-type=") {
				newArgs = append(newArgs, fmt.Sprintf("--cluster-type=%s", clusterType))
			} else if strings.HasPrefix(arg, "--secret-with-kubeconfig=") {
				newArgs = append(newArgs, fmt.Sprintf("--secret-with-kubeconfig=%s", getSecretName(clusterName)))
			} else {
				newArgs = append(newArgs, arg) // Keep other arguments as they are
			}
		}
		depl.Spec.Template.Spec.Containers[i].Args = newArgs
		found = true
		break
	}

	if !found {
		msg := "Error: 'controller' container not found in deployment"
		logger.V(logs.LogDebug).Info(msg)
		return nil, fmt.Errorf("%s", msg)
	}

	return depl, nil
}

func createNamespace(ctx context.Context, c client.Client, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := c.Create(ctx, ns)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func createServiceAccount(ctx context.Context, c client.Client, namespace, name string) error {
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	err := c.Create(ctx, serviceAccount)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func createSecret(ctx context.Context, c client.Client, namespace, name string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				corev1.ServiceAccountNameKey: name,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}

	err := c.Create(ctx, secret)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

// createManagementClusterURLConfigMap persists the management cluster's externally reachable
// API server address (as provided via --management-cluster-url) at the same name/namespace a
// non-token pull-mode registration would otherwise use for the SA-token Secret, which --token
// mode skips creating. Not a Secret: this value isn't sensitive, and keeping it a ConfigMap
// means it can be read without Secret-read RBAC.
func createManagementClusterURLConfigMap(ctx context.Context, c client.Client, namespace, name, server string,
	caData []byte) error {

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			managementClusterURLConfigMapKey: server,
			managementClusterCAConfigMapKey:  string(caData),
		},
	}

	currentConfigMap := &corev1.ConfigMap{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)
	if err == nil {
		currentConfigMap.Data = configMap.Data
		return c.Update(ctx, currentConfigMap)
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	return c.Create(ctx, configMap)
}

func createRole(ctx context.Context, c client.Client, namespace, name string) error {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(role)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer

	if err := tmpl.Execute(&buffer,
		struct {
			Namespace, Name string
		}{
			Namespace: namespace,
			Name:      name,
		}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	instantiatedRole := buffer.String()

	uRole, err := k8s_utils.GetUnstructured([]byte(instantiatedRole))
	if err != nil {
		return err
	}

	// Permissions might change with new releases
	currentRole := &rbacv1.Role{}
	err = c.Get(ctx, types.NamespacedName{Namespace: uRole.GetNamespace(), Name: uRole.GetName()}, currentRole)
	if err == nil {
		uRole.SetResourceVersion(currentRole.ResourceVersion)
		return c.Update(ctx, uRole)
	}

	err = c.Create(ctx, uRole)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func createClusterRole(ctx context.Context, c client.Client, namespace, name string) error {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(clusterRole)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer

	if err := tmpl.Execute(&buffer,
		struct {
			Namespace, Name string
		}{
			Namespace: namespace,
			Name:      name,
		}); err != nil {
		return fmt.Errorf("error executing template: %w", err)
	}
	instantiatedClusterRole := buffer.String()

	uClusterRole, err := k8s_utils.GetUnstructured([]byte(instantiatedClusterRole))
	if err != nil {
		return err
	}

	// Permissions might change with new releases
	currentClusterRole := &rbacv1.ClusterRole{}
	err = c.Get(ctx, types.NamespacedName{Name: uClusterRole.GetName()}, currentClusterRole)
	if err == nil {
		uClusterRole.SetResourceVersion(currentClusterRole.ResourceVersion)
		return c.Update(ctx, uClusterRole)
	}

	err = c.Create(ctx, uClusterRole)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

// createTokenRenewalRole grants sveltoscluster-manager's ServiceAccount permission to renew
// the token for this cluster's ServiceAccount, and only this one: resourceNames restricts the
// grant to the ServiceAccount named after the cluster, in this cluster's namespace.
func createTokenRenewalRole(ctx context.Context, c client.Client, namespace, name string) error {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name + tokenRenewalRBACNamePostfix,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{""},
				Resources:     []string{"serviceaccounts/token"},
				ResourceNames: []string{name},
				Verbs:         []string{"create"},
			},
		},
	}

	// Permissions might change with new releases
	currentRole := &rbacv1.Role{}
	err := c.Get(ctx, types.NamespacedName{Namespace: role.Namespace, Name: role.Name}, currentRole)
	if err == nil {
		role.SetResourceVersion(currentRole.ResourceVersion)
		return c.Update(ctx, role)
	}

	err = c.Create(ctx, role)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

// createTokenRenewalRoleBinding binds the Role created by createTokenRenewalRole to
// sveltoscluster-manager's ServiceAccount. sveltosNamespace is where Sveltos is installed
// in the management cluster (sc-manager's own namespace), which is not fixed.
func createTokenRenewalRoleBinding(ctx context.Context, c client.Client, namespace, name, sveltosNamespace string) error {
	roleBindingName := name + tokenRenewalRBACNamePostfix
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      roleBindingName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     roleKind,
			Name:     roleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      serviceAccountKind,
				Namespace: sveltosNamespace,
				Name:      sveltosClusterManagerServiceAccount,
			},
		},
	}

	currentRoleBinding := &rbacv1.RoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Namespace: roleBinding.Namespace, Name: roleBinding.Name}, currentRoleBinding)
	if err == nil {
		// Subjects is immutable on update for RoleBindings; delete and recreate if it changed.
		if currentRoleBinding.Subjects[0].Namespace == sveltosNamespace {
			return nil
		}
		if err := c.Delete(ctx, currentRoleBinding); err != nil {
			return err
		}
	}

	err = c.Create(ctx, roleBinding)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func createRoleBinding(ctx context.Context, c client.Client, namespace, name string) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     roleKind,
			Name:     name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      serviceAccountKind,
				Namespace: namespace,
				Name:      name,
			},
		},
	}

	err := c.Create(ctx, roleBinding)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return nil
}

func createClusterRoleBinding(ctx context.Context, c client.Client, namespace, name string) error {
	// This binds serviceAccount with clusterRole. This grants read permissions for
	// resources like Classifier
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace + "-" + name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     "ClusterRole",
			Name:     namespace + "-" + name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      serviceAccountKind,
				Namespace: namespace,
				Name:      name,
			},
		},
	}

	err := c.Create(ctx, clusterRoleBinding)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func updateSveltosClusterLabelsAndAnnotations(ctx context.Context, c client.Client,
	sveltosCluster *libsveltosv1beta1.SveltosCluster, labels map[string]string,
	shard string) error {

	lbls := sveltosCluster.Labels
	if lbls == nil {
		lbls = map[string]string{}
	}

	for k := range labels {
		lbls[k] = labels[k]
	}

	sveltosCluster.Labels = lbls

	if shard != "" {
		sveltosCluster.Annotations = map[string]string{
			shardingAnnotationKey: shard,
		}
	} else if sveltosCluster.Annotations != nil {
		delete(sveltosCluster.Annotations, shardingAnnotationKey)
	}

	return c.Update(ctx, sveltosCluster)
}

func createSveltosCluster(ctx context.Context, c client.Client, namespace, name, shard string,
	labels map[string]string, tokenRenewal bool) error {

	currentSveltosCluster := &libsveltosv1beta1.SveltosCluster{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, currentSveltosCluster)
	if err == nil {
		// Update labels
		return updateSveltosClusterLabelsAndAnnotations(ctx, c, currentSveltosCluster, labels, shard)
	}

	sveltosCluster := &libsveltosv1beta1.SveltosCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: libsveltosv1beta1.SveltosClusterSpec{
			PullMode: true,
		},
	}

	if tokenRenewal {
		sveltosCluster.Spec.TokenRequestRenewalOption = &libsveltosv1beta1.TokenRequestRenewalOption{
			RenewTokenRequestInterval: metav1.Duration{Duration: pullModeTokenRenewalInterval},
			TokenDuration:             metav1.Duration{Duration: pullModeTokenDuration},
			SANamespace:               namespace,
			SAName:                    name,
		}
	}

	if shard != "" {
		sveltosCluster.Annotations = map[string]string{
			shardingAnnotationKey: shard,
		}
	}

	err = c.Create(ctx, sveltosCluster)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
	}

	return err
}

func getKubeconfig(ctx context.Context, c client.Client,
	namespace, name, server string) (string, error) {

	secret := &corev1.Secret{}

	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret)
	if err != nil {
		return "", err
	}

	token, err := getToken(secret)
	if err != nil {
		return "", err
	}
	caCrt, err := getCaCrt(secret)
	if err != nil {
		return "", err
	}

	return getKubeconfigFromToken(server, token, caCrt), nil
}

// getKubeconfigFromTokenRequest requests a token for the cluster's ServiceAccount via the
// TokenRequest API (instead of reading a long-lived Secret), and builds a kubeconfig from it.
// The SveltosCluster's TokenRequestRenewalOption, set by createSveltosCluster, keeps this
// token renewed going forward.
func getKubeconfigFromTokenRequest(ctx context.Context, config *rest.Config, server,
	namespace, name string, logger logr.Logger) (string, error) {

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	expirationSeconds := int64(pullModeTokenDuration.Seconds())
	treq := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			ExpirationSeconds: &expirationSeconds,
		},
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Create Token for ServiceAccount %s/%s", namespace, name))
	tokenRequest, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, name, treq, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	return getKubeconfigFromToken(server, []byte(tokenRequest.Status.Token), config.CAData), nil
}

func getToken(secret *corev1.Secret) ([]byte, error) {
	if secret.Data == nil {
		return nil, errors.New("secret data is nil")
	}

	token, ok := secret.Data["token"]
	if !ok {
		return nil, errors.New("secret data does not contain token key")
	}

	return token, nil
}

func getCaCrt(secret *corev1.Secret) ([]byte, error) {
	if secret.Data == nil {
		return nil, errors.New("secret data is nil")
	}

	caCrt, ok := secret.Data["ca.crt"]
	if !ok {
		return nil, errors.New("secret data does not contain ca.crt key")
	}

	return caCrt, nil
}

func getKubeconfigFromToken(server string, token, caData []byte) string {
	configTemplate := `apiVersion: v1
kind: Config
clusters:
- name: local
  cluster:
    server: %s
    certificate-authority-data: %s
users:
- name: sveltos-applier
  user:
    token: %s
contexts:
- name: sveltos-context
  context:
    cluster: local
    user: sveltos-applier
current-context: sveltos-context`

	caDataBase64 := base64.StdEncoding.EncodeToString(caData)
	tokenString := string(token) // Token is already in the correct format

	data := fmt.Sprintf(configTemplate, server, caDataBase64, tokenString)

	return data
}

var (
	role = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
rules:
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - configurationgroups
  verbs:
  - get
  - list
  - watch
  - update
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - configurationgroups/status
  verbs:
  - get
  - update
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - configurationbundles
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - config.projectsveltos.io
  resources:
  - clusterconfigurations
  - clustersummaries
  verbs:
  - get
  - list
  - update
  - watch
- apiGroups:
  - config.projectsveltos.io
  resources:
  - clusterreports
  verbs:
  - create
  - delete
  - get
  - list
  - update
  - watch
- apiGroups:
  - config.projectsveltos.io
  resources:
  - clusterconfigurations/status
  - clusterreports/status
  - clustersummaries/status
  verbs:
  - get
  - list
  - update
  - patch
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - sveltosclusters
  verbs:
  - get
  - list
  - watch
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - sveltosclusters/status
  verbs:
  - get
  - list
  - update
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - resourcesummaries
  verbs:
  - get
  - list
  - create
  - watch
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - resourcesummaries/status
  verbs:
  - get
  - update
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - classifierreports
  - eventreports
  - healthcheckreports
  - reloaderreports
  verbs:
  - create
  - get
  - list
  - update
  - watch
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - classifierreports/status
  - eventreports/status
  - healthcheckreports/status
  - reloaderreports/status
  verbs:
  - get
  - update
  - patch
`

	clusterRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {{ .Namespace }}-{{ .Name }}
rules:
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - classifiers
  - eventsources
  - healthchecks
  verbs:
  - get
  - list
  - watch
`
)

func prepareApplierYAML(kubeconfig, clusterNamespace, clusterName string,
	logger logr.Logger) (string, error) {

	applierYAML := agent.GetSveltosAgentYAML()

	elements, err := deployer.CustomSplit(string(applierYAML))
	if err != nil {
		return "", err
	}

	var final string
	const separator = "---\n"

	for i := range elements {
		policy, err := k8s_utils.GetUnstructured([]byte(elements[i]))
		if err != nil {
			logger.V(logs.LogInfo).Info(fmt.Sprintf("failed to parse applier yaml: %v", err))
			return "", err
		}

		if policy.GetKind() == "Deployment" {
			depl := &appsv1.Deployment{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(policy.Object, depl); err != nil {
				return "", err
			}

			depl, err = modifyDeployment(depl, clusterNamespace, clusterName, logger)
			if err != nil {
				return "", err
			}

			unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&depl)
			if err != nil {
				logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to convert deployment instance to unstructured: %v", err))
				return "", err
			}

			policy.SetUnstructuredContent(unstructuredObj)
		}

		resourceYAML, err := getYAMLFromUnstructured(policy)
		if err != nil {
			return "", err
		}
		final += separator
		final += resourceYAML
	}

	// Finally create a Secret with Kubeconfig to access the management cluster
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "projectsveltos",
			Name:      getSecretName(clusterName),
		},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfig),
		},
	}

	unstructuredObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&secret)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to convert secret instance to unstructured: %v", err))
		return "", err
	}
	policy := &unstructured.Unstructured{}
	policy.SetUnstructuredContent(unstructuredObj)

	resourceYAML, err := getYAMLFromUnstructured(policy)
	if err != nil {
		return "", err
	}

	final += separator
	final += resourceYAML

	return final, nil
}

// getYAMLFromUnstructured converts an *unstructured.Unstructured object to its YAML string representation.
func getYAMLFromUnstructured(obj *unstructured.Unstructured) (string, error) {
	if obj == nil {
		return "", fmt.Errorf("input unstructured object is nil")
	}

	// 1. Convert Unstructured object to JSON bytes
	jsonBytes, err := obj.MarshalJSON()
	if err != nil {
		return "", fmt.Errorf("failed to marshal unstructured object to JSON: %w", err)
	}

	// 2. Convert JSON bytes to YAML bytes
	yamlBytes, err := yaml.JSONToYAML(jsonBytes)
	if err != nil {
		return "", fmt.Errorf("failed to convert JSON to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

func getSecretName(clusterName string) string {
	return clusterName + "-sveltos-kubeconfig"
}
