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

package redeploy

import (
	"context"
	"flag"
	"fmt"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// ForceDeployment initiates a cluster-wide redeployment of all add-ons and applications
// managed by Sveltos for the target cluster.
//
// This function internally clears the Status field of all relevant ClusterSummary
// instances associated with the SveltosCluster resource. Clearing the status forces
// the Sveltos controllers to re-evaluate and re-apply all deployed resources,
// treating them as requiring a fresh reconciliation, thus bypassing content hashing
// checks. This action is irreversible once executed.
func ForceDeployment(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl redeploy cluster [options] --namespace=<name> --cluster=<name> --cluster-type=<type> [--verbose]

     --namespace=<name>     Specifies the namespace where the Cluster resource is located.
     --cluster=<name>       Defines the name of the target cluster to force redeployment on.
     --cluster-type=<type>  Specifies the type of cluster. Accepted values are 'Capi' and 'Sveltos'.

Options:
  -h --help                Show this screen.
     --verbose             Verbose mode. Print each step.

Description:
  The 'sveltosctl redeploy cluster' command forces Sveltos to re-apply all
  configured add-ons and resources for the specified cluster. This is achieved by
  resetting the cluster's internal reconciliation status, compelling the Addon Controller
  to immediately re-process all associated ClusterProfile/Profile configurations.

  Use this command to trigger a rolling update or configuration re-application
  without making any changes to the ClusterProfile/Profile Spec.
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

	var clusterType libsveltosv1beta1.ClusterType
	if passedClusterType := parsedArgs["--cluster-type"]; passedClusterType != nil {
		switch passedClusterType {
		case string(libsveltosv1beta1.ClusterTypeCapi):
			clusterType = libsveltosv1beta1.ClusterTypeCapi
		case string(libsveltosv1beta1.ClusterTypeSveltos):
			clusterType = libsveltosv1beta1.ClusterTypeSveltos
		default:
			return fmt.Errorf("invalid cluster type: %s. Accepted values are '%s' and '%s'",
				passedClusterType,
				libsveltosv1beta1.ClusterTypeCapi,
				libsveltosv1beta1.ClusterTypeSveltos)
		}
	}

	if namespace == "" || cluster == "" {
		return fmt.Errorf("both --namespace and --cluster must be specified")
	}

	return resetClusterSummaryInstance(ctx, namespace, cluster, &clusterType, logger)
}

// resetClusterSummaryInstance finds all ClusterSummary resources associated with
// the given cluster and resets their Status field to force a full redeployment.
func resetClusterSummaryInstance(ctx context.Context, namespace, cluster string,
	clusterType *libsveltosv1beta1.ClusterType, logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(
		"Preparing to force redeployment by resetting ClusterSummary statuses",
		"Namespace", namespace, "Cluster", cluster,
	)

	// 1. Get the Kubernetes Client
	// Access the initialized client based on the user's kubeconfig context.
	instance := utils.GetAccessInstance()
	c := instance.GetClient()

	// Log if the client is not initialized (shouldn't happen if sveltosctl is set up correctly)
	if c == nil {
		return fmt.Errorf("failed to get Kubernetes client: client is not initialized")
	}
	// 2. Get ClusterSummaries in Dependency Order
	resetOrder, csMap, err := getClusterSummariesInOrder(ctx, c, namespace, cluster, clusterType)
	if err != nil {
		return err
	}

	if len(resetOrder) == 0 {
		logger.V(logs.LogDebug).Info("No ClusterSummary instances found matching the cluster criteria. Nothing to reset.")
		return nil
	}

	logger.V(logs.LogDebug).Info("ClusterSummary reset order determined", "Order", resetOrder)

	// 3. Execute the Status Reset in the Determined Order
	return performStatusReset(ctx, c, resetOrder, csMap, logger)
}

// getClusterSummariesInOrder lists all relevant ClusterSummary instances,
// constructs a dependency graph based on Spec.DependsOn, performs a topological
// sort, and returns the list of names in the required reset order along with
// a map of the objects.
//
// The order is determined such that if B depends on A, A is reset before B.
func getClusterSummariesInOrder(ctx context.Context, c client.Client, namespace, cluster string,
	clusterType *libsveltosv1beta1.ClusterType,
) (resetOrder []string, csMap map[string]*configv1beta1.ClusterSummary, err error) {

	// 2. List all ClusterSummary resources
	clusterSummaryList := &configv1beta1.ClusterSummaryList{}

	listOptions := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{
			configv1beta1.ClusterNameLabel: cluster,
			configv1beta1.ClusterTypeLabel: string(*clusterType),
		},
	}

	if err := c.List(ctx, clusterSummaryList, listOptions...); err != nil {
		return nil, nil, fmt.Errorf("failed to list ClusterSummary instances: %w", err)
	}

	if len(clusterSummaryList.Items) == 0 {
		return nil, nil, nil
	}

	// --- Graph Construction ---

	// Map: CS Name -> Set of CS Names that it depends on (outgoing dependencies)
	dependencies := make(map[string]map[string]bool)
	// Map: CS Name -> Pointer to the actual ClusterSummary object
	csMap = make(map[string]*configv1beta1.ClusterSummary)

	// Initialize maps and populate dependencies
	for i := range clusterSummaryList.Items {
		cs := &clusterSummaryList.Items[i]
		csMap[cs.Name] = cs
		dependencies[cs.Name] = make(map[string]bool)
	}

	for i := range clusterSummaryList.Items {
		cs := &clusterSummaryList.Items[i]
		for j := range cs.Spec.ClusterProfileSpec.DependsOn {
			depName := cs.Spec.ClusterProfileSpec.DependsOn[j]

			// Only consider dependencies that are within the currently listed set (i.e., local to this cluster)
			if _, exists := csMap[depName]; exists {
				dependencies[cs.Name][depName] = true
			}
		}
	}

	// --- Topological Sort (Kahn's Algorithm for Reset Order) ---

	// Queue for resources with zero *OUTGOING* dependencies (the last items in the chain)
	queue := []string{}
	// Map of outgoing dependencies count
	outgoingCount := make(map[string]int)

	for name, outgoingDeps := range dependencies {
		outgoingCount[name] = len(outgoingDeps)
		if len(outgoingDeps) == 0 {
			queue = append(queue, name)
		}
	}

	resetOrder = []string{}

	for len(queue) > 0 {
		csName := queue[0]
		queue = queue[1:]

		resetOrder = append(resetOrder, csName)

		// Find all ClusterSummaries that depended *on* this one (incoming dependencies)
		for dependentName, dependentDeps := range dependencies {
			// Check if 'dependentName' depends on 'csName'
			if _, ok := dependentDeps[csName]; ok {
				// This means dependentName relies on csName.
				// We are removing csName, so dependentName loses one dependency.
				outgoingCount[dependentName]--

				if outgoingCount[dependentName] == 0 {
					queue = append(queue, dependentName)
				}
			}
		}
	}

	if len(resetOrder) != len(csMap) {
		// Cycle detected
		return nil, nil,
			fmt.Errorf(
				"dependency cycle detected in ClusterSummary resources for cluster %s/%s. Cannot proceed with safe redeployment",
				namespace, cluster)
	}

	return resetOrder, csMap, nil
}

// performStatusReset iterates through the ClusterSummary resources in the provided
// order and clears their Status field via a Patch operation.
func performStatusReset(ctx context.Context, c client.Client, resetOrder []string,
	csMap map[string]*configv1beta1.ClusterSummary, logger logr.Logger) error {

	for _, csName := range resetOrder {
		cs := csMap[csName]

		// Create an empty ClusterSummaryStatus to patch
		emptyStatus := configv1beta1.ClusterSummaryStatus{}

		// Use Patch to clear only the status field
		// We use DeepCopy() to ensure the object passed to MergeFrom is the original state.
		patch := client.MergeFrom(cs.DeepCopy())
		cs.Status = emptyStatus

		logger.V(logs.LogDebug).Info("Attempting to patch ClusterSummary status", "ClusterSummary", cs.Name)

		if err := c.Status().Patch(ctx, cs, patch); err != nil {
			return fmt.Errorf("failed to patch ClusterSummary %s/%s status: %w", cs.Namespace, cs.Name, err)
		}

		logger.V(logs.LogDebug).Info("Successfully reset status to force redeployment", "ClusterSummary", cs.Name)
	}

	logger.V(logs.LogDebug).Info(
		"Force redeployment initiated successfully by clearing all relevant ClusterSummary statuses in dependency order.",
	)
	return nil
}
