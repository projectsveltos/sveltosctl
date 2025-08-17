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

package show

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/libsveltos/lib/k8s_utils"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// resourceKind indentifies the type of resource (ClusterProfile, ConfigMap, Secret)
	// resourceNamespace and resourceName is the kubernetes resource namespace/name
	// clusters is the list of clusters where resource content is deployed
	genAdminRbac = func(clusterKind, clusterNamespace, clusterName, serviceAccountNamespace, serviceAccountName,
		namespace, apigroups, resources, resourceNames, verbs string,
	) []string {
		return []string{
			fmt.Sprintf("%s:%s/%s", clusterKind, clusterNamespace, clusterName),
			fmt.Sprintf("%s/%s", serviceAccountNamespace, serviceAccountName),
			namespace,
			apigroups,
			resources,
			resources,
			verbs,
		}
	}
)

func displayAdminRbacs(ctx context.Context,
	passedNamespace, passedCluster, passedServiceAccountNamespace, passedServiceAccountName string,
	logger logr.Logger) error {

	// Collect all RoleRequest
	instance := utils.GetAccessInstance()

	logger.V(logs.LogDebug).Info("collect all rolerequests")
	roleRequests, err := instance.ListRoleRequests(ctx, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("found %d roleRequests", len(roleRequests.Items)))

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "ADMIN", "NAMESPACE", "API GROUPS", "RESOURCES", "RESOURCE NAMES", "VERBS")

	// Build a map: key is the cluster, value is the slices of rolerequests matching that cluster
	clusterMap := createRoleRequestsPerClusterMap(roleRequests, logger)

	for k := range clusterMap {
		l := logger.WithValues("cluster", fmt.Sprintf("%s:%s/%s", k.Kind, k.Namespace, k.Name))
		l.V(logs.LogDebug).Info("considering cluster")
		err = parseCluster(ctx, &k, clusterMap[k], passedNamespace, passedCluster, passedServiceAccountNamespace,
			passedServiceAccountName, table, l)
		if err != nil {
			return err
		}
	}

	table.Render()

	return nil
}

func createRoleRequestsPerClusterMap(roleRequests *libsveltosv1beta1.RoleRequestList,
	logger logr.Logger) map[corev1.ObjectReference][]*libsveltosv1beta1.RoleRequest {

	clusterMap := make(map[corev1.ObjectReference][]*libsveltosv1beta1.RoleRequest)

	for i := range roleRequests.Items {
		rr := &roleRequests.Items[i]
		clusterMap = parseMatchihgCluster(rr, clusterMap, logger)
	}

	return clusterMap
}

func parseMatchihgCluster(rr *libsveltosv1beta1.RoleRequest,
	clusterMap map[corev1.ObjectReference][]*libsveltosv1beta1.RoleRequest, logger logr.Logger,
) map[corev1.ObjectReference][]*libsveltosv1beta1.RoleRequest {

	logger = logger.WithValues("rolerequest", rr.Name)
	logger.V(logs.LogDebug).Info("parsing matching clusters for roleRequets")
	for i := range rr.Status.MatchingClusterRefs {
		if _, ok := clusterMap[rr.Status.MatchingClusterRefs[i]]; !ok {
			clusterMap[rr.Status.MatchingClusterRefs[i]] = make([]*libsveltosv1beta1.RoleRequest, 0)
		}
		clusterMap[rr.Status.MatchingClusterRefs[i]] = append(clusterMap[rr.Status.MatchingClusterRefs[i]], rr)
	}
	return clusterMap
}

func parseCluster(ctx context.Context, cluster *corev1.ObjectReference,
	roleRequests []*libsveltosv1beta1.RoleRequest,
	passedNamespace, passedCluster, passedServiceAccountNamespace, passedServiceAccountName string,
	table *tablewriter.Table, logger logr.Logger) error {

	if passedNamespace == "" || passedNamespace == cluster.Namespace {
		if passedCluster == "" || passedCluster == cluster.Name {
			logger.V(logs.LogDebug).Info("examining admin rbacs in cluster")
			for i := range roleRequests {
				if err := parseRoleRequest(ctx, roleRequests[i], cluster.Namespace,
					cluster.Name, cluster.Kind, passedServiceAccountNamespace, passedServiceAccountName,
					table, logger); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func shouldParseRoleRequest(roleRequest *libsveltosv1beta1.RoleRequest,
	passedServiceAccountNamespace, passedServiceAccountName string) bool {

	if passedServiceAccountNamespace != "" {
		if passedServiceAccountNamespace != roleRequest.Spec.ServiceAccountNamespace {
			return false
		}
	}

	if passedServiceAccountName != "" {
		if passedServiceAccountName != roleRequest.Spec.ServiceAccountName {
			return false
		}
	}

	return true
}

func parseRoleRequest(ctx context.Context, roleRequest *libsveltosv1beta1.RoleRequest,
	clusterNamespace, clusterName, clusterKind, passedServiceAccountNamespace, passedServiceAccountName string,
	table *tablewriter.Table, logger logr.Logger) error {

	logger = logger.WithValues("admin", fmt.Sprintf("%s/%s",
		roleRequest.Spec.ServiceAccountNamespace, roleRequest.Spec.ServiceAccountName))
	logger.V(logs.LogDebug).Info("considering rolerequest %s", roleRequest.Name)
	if shouldParseRoleRequest(roleRequest, passedServiceAccountNamespace, passedServiceAccountName) {
		logger.V(logs.LogDebug).Info("rolerequest is for admin")
		for i := range roleRequest.Spec.RoleRefs {
			if err := parseReferencedResource(ctx, clusterNamespace, clusterName, clusterKind,
				roleRequest.Spec.ServiceAccountNamespace, roleRequest.Spec.ServiceAccountName,
				roleRequest.Spec.RoleRefs[i], table, logger); err != nil {
				return err
			}
		}
	}

	return nil
}

func parseReferencedResource(ctx context.Context,
	clusterNamespace, clusterName, clusterKind, serviceAccountNamespace, serviceAccountName string,
	resource libsveltosv1beta1.PolicyRef, table *tablewriter.Table, logger logr.Logger) error {

	// fetch resource
	content, err := collectResourceContent(ctx, resource, logger)
	if err != nil {
		return err
	}

	for i := range content {
		if content[i].GroupVersionKind().Kind == "Role" {
			err = processRole(content[i], clusterNamespace, clusterName, clusterKind,
				serviceAccountNamespace, serviceAccountName, table, logger)
			if err != nil {
				return err
			}
		} else if content[i].GroupVersionKind().Kind == "ClusterRole" {
			err = processClusterRole(content[i], clusterNamespace, clusterName, clusterKind,
				serviceAccountNamespace, serviceAccountName, table, logger)
			if err != nil {
				return err
			}
		} else {
			logger.V(logs.LogDebug).Info("resource is neither Role or ClusterRole")
		}
	}

	return nil
}

func processRole(u *unstructured.Unstructured,
	clusterNamespace, clusterName, clusterKind, serviceAccountNamespace, serviceAccountName string,
	table *tablewriter.Table, logger logr.Logger) error {

	role := &rbacv1.Role{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), role); err != nil {
		return err
	}
	logger = logger.WithValues("role", fmt.Sprintf("%s/%s", role.Namespace, role.Name))
	logger.V(logs.LogDebug).Info("process role")

	for i := range role.Rules {
		rule := role.Rules[i]
		resourceNames := ""
		if rule.ResourceNames != nil {
			resourceNames = strings.Join(rule.ResourceNames, ",")
		}

		table.Append(genAdminRbac(clusterKind, clusterNamespace, clusterName, serviceAccountNamespace,
			serviceAccountName, role.Namespace, strings.Join(rule.APIGroups, ","), strings.Join(rule.Resources, ","),
			resourceNames, strings.Join(rule.Verbs, ",")))
	}

	return nil
}

func processClusterRole(u *unstructured.Unstructured,
	clusterNamespace, clusterName, clusterKind, serviceAccountNamespace, serviceAccountName string,
	table *tablewriter.Table, logger logr.Logger) error {

	clusterRole := &rbacv1.ClusterRole{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.UnstructuredContent(), clusterRole); err != nil {
		return err
	}
	logger = logger.WithValues("role", fmt.Sprintf("%s/%s", clusterRole.Namespace, clusterRole.Name))
	logger.V(logs.LogDebug).Info("process role")

	for i := range clusterRole.Rules {
		rule := clusterRole.Rules[i]
		resourceNames := ""
		if rule.ResourceNames != nil {
			resourceNames = strings.Join(rule.ResourceNames, ",")
		}

		table.Append(genAdminRbac(clusterKind, clusterNamespace, clusterName, serviceAccountNamespace, serviceAccountName,
			"*", strings.Join(rule.APIGroups, ","), strings.Join(rule.Resources, ","),
			resourceNames, strings.Join(rule.Verbs, ",")))
	}

	return nil
}

func collectResourceContent(ctx context.Context, resource libsveltosv1beta1.PolicyRef, logger logr.Logger,
) ([]*unstructured.Unstructured, error) {

	logger = logger.WithValues("kind", resource.Kind,
		"resource", fmt.Sprintf("%s/%s", resource.Namespace, resource.Name))
	logger.V(logs.LogDebug).Info("collect resource")
	instance := utils.GetAccessInstance()
	if resource.Kind == string(libsveltosv1beta1.ConfigMapReferencedResourceKind) {
		configMap := &corev1.ConfigMap{}
		err := instance.GetResource(ctx,
			types.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}, configMap)
		if err != nil {
			return nil, err
		}
		return collectContent(configMap.Data, logger)
	}

	secret := &corev1.Secret{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.Namespace, Name: resource.Name}, secret)
	if err != nil {
		return nil, err
	}
	data := make(map[string]string)
	for key, value := range secret.Data {
		data[key] = string(value)
	}
	return collectContent(data, logger)
}

func collectContent(data map[string]string, logger logr.Logger) ([]*unstructured.Unstructured, error) {
	policies := make([]*unstructured.Unstructured, 0)

	const separator = "---"
	for k := range data {
		elements := strings.Split(data[k], separator)
		for i := range elements {
			if elements[i] == "" {
				continue
			}

			policy, err := k8s_utils.GetUnstructured([]byte(elements[i]))
			if err != nil {
				logger.Error(err, fmt.Sprintf("failed to get policy from Data %.100s", elements[i]))
				return nil, err
			}

			if policy == nil {
				logger.Error(err, fmt.Sprintf("failed to get policy from Data %.100s", elements[i]))
				return nil, fmt.Errorf("failed to get policy from Data %.100s", elements[i])
			}

			policies = append(policies, policy)
		}
	}

	return policies, nil
}

// AdminPermissions displays information about permissions each admin has in each managed cluster
func AdminPermissions(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show admin-rbac [options] [--namespace=<name>] [--cluster=<name>] [--serviceAccountName=<name>] [--serviceAccountNamespace=<name>] [--verbose]

     --serviceAccountName=<name>            Show permissions for this ServiceAccount.
                                            If not specified all admins are considered.
     --serviceAccountNamespace=<namespace>  Show permissions for this ServiceAccounts in this namespace.
                                            If not specified all namespaces are considered.
     --namespace=<name>                     Show serviceAccount permissions in clusters in this namespace.
                                            If not specified all namespaces are considered.
     --cluster=<name>                       Show serviceAccount permissions in cluster with name.
                                            If not specified all cluster names are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show admin-rbac command shows information admin's permissions in managed clusters.
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

	saName := ""
	if passedSaName := parsedArgs["--serviceAccountName"]; passedSaName != nil {
		saName = passedSaName.(string)
	}

	saNamespace := ""
	if passedSaNamespace := parsedArgs["--serviceAccountNamespace"]; passedSaNamespace != nil {
		saNamespace = passedSaNamespace.(string)
	}

	return displayAdminRbacs(ctx, namespace, cluster, saNamespace, saName, logger)
}
