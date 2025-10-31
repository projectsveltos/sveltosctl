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
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// DeregisterCluster takes care of removing all resources created during cluster registration
func DeregisterCluster(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl deregister cluster [options] --namespace=<name> --cluster=<name> [--verbose]

     --namespace=<name>     Specifies the namespace where the SveltosCluster resource is located.
     --cluster=<name>       Defines the name of the registered cluster to remove.

Options:
  -h --help                Show this screen.
     --verbose             Verbose mode. Print each step.

Description:
  The deregister cluster command removes a cluster that was previously registered with Sveltos.
  This command will delete:
  - The SveltosCluster resource
  - The associated kubeconfig Secret
  - For pull-mode clusters: ServiceAccount, Roles, RoleBindings, and ClusterRole/ClusterRoleBinding
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

	if namespace == "" || cluster == "" {
		return fmt.Errorf("both --namespace and --cluster must be specified")
	}

	return deregisterSveltosCluster(ctx, namespace, cluster, logger)
}

func deregisterSveltosCluster(ctx context.Context, clusterNamespace, clusterName string,
	logger logr.Logger,
) error {

	instance := utils.GetAccessInstance()
	c := instance.GetClient()

	// Check if SveltosCluster exists and determine if it's pull-mode
	sveltosCluster := &libsveltosv1beta1.SveltosCluster{}
	err := c.Get(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogInfo).Info(fmt.Sprintf(
				"SveltosCluster %s/%s not found. Attempting to clean up any orphaned resources.",
				clusterNamespace, clusterName))
			//nolint: forbidigo // print info message
			fmt.Printf("SveltosCluster %s/%s not found. Attempting to clean up any orphaned resources.\n",
				clusterNamespace, clusterName)

			// Even if the SveltosCluster is gone, try to clean up the kubeconfig secret
			deletedResources := []string{}
			secretName := clusterName + sveltosKubeconfigSecretNamePostfix
			if err := deleteSecret(ctx, c, clusterNamespace, secretName, logger); err != nil {
				logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete kubeconfig Secret: %v", err))
			} else {
				deletedResources = append(deletedResources,
					fmt.Sprintf("Secret/%s/%s", clusterNamespace, secretName))
			}

			if len(deletedResources) > 0 {
				//nolint: forbidigo // print deleted resources
				fmt.Printf("\nCleaned up orphaned resources:\n")
				for _, resource := range deletedResources {
					//nolint: forbidigo // print each resource
					fmt.Printf("  - %s\n", resource)
				}
			} else {
				//nolint: forbidigo // print info message
				fmt.Printf("No orphaned resources found.\n")
			}

			return nil
		}
		return fmt.Errorf("failed to get SveltosCluster: %w", err)
	}

	isPullMode := sveltosCluster.Spec.PullMode
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Cluster %s/%s is in pull-mode: %t",
		clusterNamespace, clusterName, isPullMode))

	deletedResources := []string{}

	// Delete pull-mode specific resources
	if isPullMode {
		pullModeResources := deletePullModeResources(ctx, c, clusterNamespace, clusterName, logger)
		deletedResources = append(deletedResources, pullModeResources...)
	}

	// Delete common resources (kubeconfig secret)
	secretResources := deletePushModeResources(ctx, c, clusterNamespace, clusterName, logger)
	deletedResources = append(deletedResources, secretResources...)

	// Delete SveltosCluster
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting SveltosCluster %s/%s", clusterNamespace, clusterName))
	if err := instance.DeleteResource(ctx, sveltosCluster); err != nil {
		return fmt.Errorf("failed to delete SveltosCluster: %w", err)
	}
	deletedResources = append(deletedResources,
		fmt.Sprintf("SveltosCluster/%s/%s", clusterNamespace, clusterName))

	//nolint: forbidigo // print success message
	fmt.Printf("Successfully deregistered cluster %s/%s\n", clusterNamespace, clusterName)
	//nolint: forbidigo // print deleted resources
	fmt.Printf("\nDeleted resources:\n")
	for _, resource := range deletedResources {
		//nolint: forbidigo // print each resource
		fmt.Printf("  - %s\n", resource)
	}

	return nil
}

// deletePullModeResources removes all pull-mode specific resources
// Returns a list of successfully deleted resources
func deletePullModeResources(ctx context.Context, c client.Client, clusterNamespace, clusterName string,
	logger logr.Logger,
) []string {

	deletedResources := []string{}

	// Delete ClusterRoleBinding
	if err := deleteClusterRoleBinding(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete ClusterRoleBinding: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("ClusterRoleBinding/%s-%s", clusterNamespace, clusterName))
	}

	// Delete ClusterRole
	if err := deleteClusterRole(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete ClusterRole: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("ClusterRole/%s-%s", clusterNamespace, clusterName))
	}

	// Delete RoleBinding
	if err := deleteRoleBinding(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete RoleBinding: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("RoleBinding/%s/%s", clusterNamespace, clusterName))
	}

	// Delete Role
	if err := deleteRole(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete Role: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("Role/%s/%s", clusterNamespace, clusterName))
	}

	// Delete ServiceAccount Secret
	if err := deleteSecret(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete ServiceAccount Secret: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("Secret/%s/%s", clusterNamespace, clusterName))
	}

	// Delete ServiceAccount
	if err := deleteServiceAccount(ctx, c, clusterNamespace, clusterName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete ServiceAccount: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("ServiceAccount/%s/%s", clusterNamespace, clusterName))
	}

	return deletedResources
}

// deletePushModeResources removes resources that exist in push mode (just kubeconfig secret)
// This is common to both push and pull mode clusters
// Returns a list of successfully deleted resources
func deletePushModeResources(ctx context.Context, c client.Client, clusterNamespace, clusterName string,
	logger logr.Logger,
) []string {

	deletedResources := []string{}

	// Delete kubeconfig Secret
	secretName := clusterName + sveltosKubeconfigSecretNamePostfix
	if err := deleteSecret(ctx, c, clusterNamespace, secretName, logger); err != nil {
		logger.V(logs.LogInfo).Info(fmt.Sprintf("Warning: failed to delete kubeconfig Secret: %v", err))
	} else {
		deletedResources = append(deletedResources,
			fmt.Sprintf("Secret/%s/%s", clusterNamespace, secretName))
	}

	return deletedResources
}

func deleteServiceAccount(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	serviceAccount := &corev1.ServiceAccount{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, serviceAccount)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("ServiceAccount %s/%s not found", namespace, name))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting ServiceAccount %s/%s", namespace, name))
	return c.Delete(ctx, serviceAccount)
}

func deleteSecret(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Secret %s/%s not found", namespace, name))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting Secret %s/%s", namespace, name))
	return c.Delete(ctx, secret)
}

func deleteRole(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	role := &rbacv1.Role{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, role)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Role %s/%s not found", namespace, name))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting Role %s/%s", namespace, name))
	return c.Delete(ctx, role)
}

func deleteClusterRole(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	clusterRoleName := namespace + "-" + name
	clusterRole := &rbacv1.ClusterRole{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleName}, clusterRole)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("ClusterRole %s not found", clusterRoleName))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting ClusterRole %s", clusterRoleName))
	return c.Delete(ctx, clusterRole)
}

func deleteRoleBinding(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	roleBinding := &rbacv1.RoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, roleBinding)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("RoleBinding %s/%s not found", namespace, name))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting RoleBinding %s/%s", namespace, name))
	return c.Delete(ctx, roleBinding)
}

func deleteClusterRoleBinding(ctx context.Context, c client.Client, namespace, name string,
	logger logr.Logger,
) error {

	clusterRoleBindingName := namespace + "-" + name
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	err := c.Get(ctx, types.NamespacedName{Name: clusterRoleBindingName}, clusterRoleBinding)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("ClusterRoleBinding %s not found", clusterRoleBindingName))
			return nil
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Deleting ClusterRoleBinding %s", clusterRoleBindingName))
	return c.Delete(ctx, clusterRoleBinding)
}
