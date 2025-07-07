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

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/deployer"
	"github.com/projectsveltos/libsveltos/lib/k8s_utils"
	"github.com/projectsveltos/libsveltos/lib/logsettings"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/agent"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func onboardSveltosClusterInPullMode(ctx context.Context, clusterNamespace, clusterName string,
	labels map[string]string, logger logr.Logger) error {

	instance := utils.GetAccessInstance()
	c := instance.GetClient()

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

	err = createSecret(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createSecret failed: %s", err))
		return err
	}

	err = createRole(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRole failed: %s", err))
		return err
	}

	err = createClusterRole(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRole failed: %s", err))
		return err
	}

	err = createRoleBinding(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createRoleBinding failed: %s", err))
		return err
	}

	err = createClusterRoleBinding(ctx, c, clusterNamespace, clusterName)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createClusterRoleBinding failed: %s", err))
		return err
	}

	err = createSveltosCluster(ctx, c, clusterNamespace, clusterName, labels)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("createSveltosCluster failed: %s", err))
		return err
	}

	config := instance.GetConfig()

	kubeconfig, err := getKubeconfig(ctx, c, clusterNamespace, clusterName, config.Host)
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
	err = c.Get(ctx, types.NamespacedName{Name: uRole.GetName()}, currentRole)
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

func createRoleBinding(ctx context.Context, c client.Client, namespace, name string) error {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
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
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     namespace + "-" + name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
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

func updateSveltosClusterLabels(ctx context.Context, c client.Client, sveltosCluster *libsveltosv1beta1.SveltosCluster,
	labels map[string]string) error {

	lbls := sveltosCluster.Labels
	if lbls == nil {
		lbls = map[string]string{}
	}

	for k := range labels {
		lbls[k] = labels[k]
	}

	sveltosCluster.Labels = lbls
	return c.Update(ctx, sveltosCluster)
}

func createSveltosCluster(ctx context.Context, c client.Client, namespace, name string,
	labels map[string]string) error {

	currentSveltosCluster := &libsveltosv1beta1.SveltosCluster{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, currentSveltosCluster)
	if err == nil {
		// Update labels
		return updateSveltosClusterLabels(ctx, c, currentSveltosCluster, labels)
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
	template := `apiVersion: v1
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

	data := fmt.Sprintf(template, server, caDataBase64, tokenString)

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
  - classifiers
  verbs:
  - get
  - list
- apiGroups:
  - lib.projectsveltos.io
  resources:
  - classifierreports
  verbs:
  - create
  - get
  - list
  - update
  - watch
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
				logger.V(logsettings.LogDebug).Info(fmt.Sprintf("failed to convert deployment instance to unstructured: %v", err))
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
		logger.V(logsettings.LogDebug).Info(fmt.Sprintf("failed to convert secret instance to unstructured: %v", err))
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
