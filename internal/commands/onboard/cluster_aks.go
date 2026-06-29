/*
Copyright 2026. projectsveltos.io. All rights reserved.

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

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

func buildAKSWorkloadIdentityConfig(
	endpoint, tenantID, clientID, subscriptionID, resourceGroup, aksClusterName string,
) (*libsveltosv1beta1.WorkloadIdentityConfig, error) {

	if endpoint == "" {
		return nil, fmt.Errorf("--endpoint is required")
	}
	if tenantID == "" {
		return nil, fmt.Errorf("--tenant-id is required")
	}
	if clientID == "" {
		return nil, fmt.Errorf("--client-id is required")
	}
	wi := &libsveltosv1beta1.WorkloadIdentityConfig{
		Provider: libsveltosv1beta1.WorkloadIdentityProviderAzure,
		Endpoint: endpoint,
		Azure: &libsveltosv1beta1.AzureWorkloadIdentityConfig{
			TenantID:       tenantID,
			ClientID:       clientID,
			SubscriptionID: subscriptionID,
			ResourceGroup:  resourceGroup,
			ClusterName:    aksClusterName,
		},
	}
	return wi, nil
}

// RegisterClusterAKS registers an Azure AKS cluster using workload identity.
func RegisterClusterAKS(ctx context.Context, args []string, logger logr.Logger) error { //nolint: funlen // per-provider command
	doc := `Usage:
  sveltosctl register cluster-aks [options] --namespace=<name> --cluster=<name> --endpoint=<url>
                                  --tenant-id=<id> --client-id=<id>
                                  [--subscription-id=<id>] [--resource-group=<group>] [--aks-cluster-name=<name>]
                                  [--ca-file=<file>] [--labels=<value>] [--shard=<key>] [--verbose]

     --namespace=<name>           Namespace where Sveltos will create the SveltosCluster resource.
     --cluster=<name>             Name for the registered cluster within Sveltos.
     --endpoint=<url>             API server endpoint of the AKS cluster (e.g. https://my-aks.hcp.eastus.azmk8s.io).
     --tenant-id=<id>             Azure AD tenant ID.
     --client-id=<id>             Client ID of the managed identity or app registration federated with
                                  the management cluster service account.
     --subscription-id=<id>       (Optional) Azure subscription containing the AKS cluster.
     --resource-group=<group>     (Optional) Azure resource group containing the AKS cluster.
     --aks-cluster-name=<name>    (Optional) AKS cluster name.
     --ca-file=<file>             (Optional) Path to the CA certificate file for the AKS API server.
                                  When provided, sveltosctl stores the CA in a Secret and references it
                                  in the SveltosCluster.
     --labels=<key1=value1,...>   (Optional) Labels for the SveltosCluster resource (comma-separated key=value pairs).
     --shard=<shard key>          (Optional) Assigns the cluster to a specific controller shard.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The register cluster-aks command registers an Azure AKS cluster using workload identity federation.
  Sveltos obtains short-lived credentials from Azure at runtime; no long-lived kubeconfig is stored.
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
	if parsedArgs["--verbose"].(bool) {
		if err := flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug)); err != nil {
			return err
		}
	}

	namespace := ""
	if v := parsedArgs["--namespace"]; v != nil {
		namespace = v.(string)
	}
	cluster := ""
	if v := parsedArgs["--cluster"]; v != nil {
		cluster = v.(string)
	}
	endpoint := ""
	if v := parsedArgs["--endpoint"]; v != nil {
		endpoint = v.(string)
	}
	tenantID := ""
	if v := parsedArgs["--tenant-id"]; v != nil {
		tenantID = v.(string)
	}
	clientID := ""
	if v := parsedArgs["--client-id"]; v != nil {
		clientID = v.(string)
	}
	subscriptionID := ""
	if v := parsedArgs["--subscription-id"]; v != nil {
		subscriptionID = v.(string)
	}
	resourceGroup := ""
	if v := parsedArgs["--resource-group"]; v != nil {
		resourceGroup = v.(string)
	}
	aksClusterName := ""
	if v := parsedArgs["--aks-cluster-name"]; v != nil {
		aksClusterName = v.(string)
	}
	caFile := ""
	if v := parsedArgs["--ca-file"]; v != nil {
		caFile = v.(string)
	}
	shard := ""
	if v := parsedArgs["--shard"]; v != nil {
		shard = v.(string)
	}
	var labels map[string]string
	if v := parsedArgs["--labels"]; v != nil {
		labels, err = stringToMap(v.(string))
		if err != nil {
			return err
		}
	}

	wi, err := buildAKSWorkloadIdentityConfig(endpoint, tenantID, clientID, subscriptionID, resourceGroup, aksClusterName)
	if err != nil {
		return err
	}
	return onboardSveltosClusterWithWorkloadIdentity(ctx, namespace, cluster, shard, wi, caFile, labels, logger)
}
