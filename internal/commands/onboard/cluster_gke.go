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

func buildGKEWorkloadIdentityConfig(endpoint, projectID, gkeClusterName, location string) (*libsveltosv1beta1.WorkloadIdentityConfig, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("--endpoint is required")
	}
	if projectID == "" {
		return nil, fmt.Errorf("--project-id is required")
	}
	if gkeClusterName == "" {
		return nil, fmt.Errorf("--gke-cluster-name is required")
	}
	if location == "" {
		return nil, fmt.Errorf("--location is required")
	}
	wi := &libsveltosv1beta1.WorkloadIdentityConfig{
		Provider: libsveltosv1beta1.WorkloadIdentityProviderGCP,
		Endpoint: endpoint,
		GCP: &libsveltosv1beta1.GCPWorkloadIdentityConfig{
			ProjectID:   projectID,
			ClusterName: gkeClusterName,
			Location:    location,
		},
	}
	return wi, nil
}

// RegisterClusterGKE registers a Google GKE cluster using workload identity.
func RegisterClusterGKE(ctx context.Context, args []string, logger logr.Logger) error { //nolint: dupl // per-provider command
	doc := `Usage:
  sveltosctl register cluster-gke [options] --namespace=<name> --cluster=<name> --endpoint=<url>
                                  --project-id=<id> --gke-cluster-name=<name> --location=<location>
                                  [--ca-file=<file>] [--labels=<value>] [--shard=<key>] [--verbose]

     --namespace=<name>           Namespace where Sveltos will create the SveltosCluster resource.
     --cluster=<name>             Name for the registered cluster within Sveltos.
     --endpoint=<url>             API server endpoint of the GKE cluster (e.g. https://34.x.x.x).
     --project-id=<id>            GCP project ID containing the GKE cluster.
     --gke-cluster-name=<name>    GKE cluster name.
     --location=<location>        GCP region or zone of the GKE cluster (e.g. us-central1-a).
     --ca-file=<file>             (Optional) Path to the CA certificate file for the GKE API server.
                                  When provided, sveltosctl stores the CA in a Secret and references it
                                  in the SveltosCluster.
     --labels=<key1=value1,...>   (Optional) Labels for the SveltosCluster resource (comma-separated key=value pairs).
     --shard=<shard key>          (Optional) Assigns the cluster to a specific controller shard.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The register cluster-gke command registers a Google GKE cluster using workload identity federation.
  Sveltos obtains short-lived credentials from GCP at runtime; no long-lived kubeconfig is stored.
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
	projectID := ""
	if v := parsedArgs["--project-id"]; v != nil {
		projectID = v.(string)
	}
	gkeClusterName := ""
	if v := parsedArgs["--gke-cluster-name"]; v != nil {
		gkeClusterName = v.(string)
	}
	location := ""
	if v := parsedArgs["--location"]; v != nil {
		location = v.(string)
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

	wi, err := buildGKEWorkloadIdentityConfig(endpoint, projectID, gkeClusterName, location)
	if err != nil {
		return err
	}
	return onboardSveltosClusterWithWorkloadIdentity(ctx, namespace, cluster, shard, wi, caFile, labels, logger)
}
