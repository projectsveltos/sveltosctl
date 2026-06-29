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

func buildEKSWorkloadIdentityConfig(endpoint, eksClusterName, roleARN, region string) (*libsveltosv1beta1.WorkloadIdentityConfig, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("--endpoint is required")
	}
	if eksClusterName == "" {
		return nil, fmt.Errorf("--eks-cluster-name is required")
	}
	wi := &libsveltosv1beta1.WorkloadIdentityConfig{
		Provider: libsveltosv1beta1.WorkloadIdentityProviderAWS,
		Endpoint: endpoint,
		AWS: &libsveltosv1beta1.AWSWorkloadIdentityConfig{
			ClusterName: eksClusterName,
			RoleARN:     roleARN,
			Region:      region,
		},
	}
	return wi, nil
}

// RegisterClusterEKS registers an Amazon EKS cluster using workload identity.
func RegisterClusterEKS(ctx context.Context, args []string, logger logr.Logger) error { //nolint: dupl // per-provider command
	doc := `Usage:
  sveltosctl register cluster-eks [options] --namespace=<name> --cluster=<name> --endpoint=<url> --eks-cluster-name=<name>
                                  [--role-arn=<arn>] [--region=<region>] [--ca-file=<file>]
                                  [--labels=<value>] [--shard=<key>] [--verbose]

     --namespace=<name>           Namespace where Sveltos will create the SveltosCluster resource.
     --cluster=<name>             Name for the registered cluster within Sveltos.
     --endpoint=<url>             API server endpoint of the EKS cluster (e.g. https://...).
     --eks-cluster-name=<name>    EKS cluster name, embedded in the bearer token sent to the EKS API server.
     --role-arn=<arn>             (Optional) IAM role ARN to assume before generating the EKS bearer token.
                                  If omitted, the pod's own IRSA credentials are used directly.
     --region=<region>            (Optional) AWS region of the EKS cluster. Defaults to the AWS_REGION
                                  environment variable injected by IRSA.
     --ca-file=<file>             (Optional) Path to the CA certificate file for the EKS API server.
                                  When provided, sveltosctl stores the CA in a Secret and references it
                                  in the SveltosCluster.
     --labels=<key1=value1,...>   (Optional) Labels for the SveltosCluster resource (comma-separated key=value pairs).
     --shard=<shard key>          (Optional) Assigns the cluster to a specific controller shard.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The register cluster-eks command registers an Amazon EKS cluster using workload identity (IRSA).
  Sveltos obtains short-lived credentials from AWS at runtime; no long-lived kubeconfig is stored.
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
	eksClusterName := ""
	if v := parsedArgs["--eks-cluster-name"]; v != nil {
		eksClusterName = v.(string)
	}
	roleARN := ""
	if v := parsedArgs["--role-arn"]; v != nil {
		roleARN = v.(string)
	}
	region := ""
	if v := parsedArgs["--region"]; v != nil {
		region = v.(string)
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

	wi, err := buildEKSWorkloadIdentityConfig(endpoint, eksClusterName, roleARN, region)
	if err != nil {
		return err
	}
	return onboardSveltosClusterWithWorkloadIdentity(ctx, namespace, cluster, shard, wi, caFile, labels, logger)
}
