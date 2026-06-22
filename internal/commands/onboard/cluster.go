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
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/commands/generate"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	//nolint: gosec // Sveltos secret postfix
	sveltosKubeconfigSecretNamePostfix = "-sveltos-kubeconfig"
	//nolint: gosec // Sveltos secret postfix
	sveltosCASecretNamePostfix = "-sveltos-ca"
	kubeconfig                 = "kubeconfig"
	caKey                      = "ca.crt"
	shardingAnnotationKey      = "sharding.projectsveltos.io/key"
	rbacAPIGroup               = "rbac.authorization.k8s.io"
	serviceAccountKind         = "ServiceAccount"
)

func onboardSveltosCluster(ctx context.Context, clusterNamespace, clusterName, shard string, kubeconfigData []byte,
	labels map[string]string, renew bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	secretName := clusterName + sveltosKubeconfigSecretNamePostfix
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Verifying Secret %s/%s does not exist already", clusterNamespace, secretName))
	secret := &corev1.Secret{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	err = patchSecret(ctx, clusterNamespace, secretName, kubeconfigData, logger)
	if err != nil {
		return err
	}

	err = patchSveltosCluster(ctx, clusterNamespace, clusterName, shard, labels, renew, logger)
	if err != nil {
		return err
	}

	//nolint: forbidigo // print success message
	fmt.Printf("cluster %s successfully registered/updated in namespace %s.", clusterName, clusterNamespace)
	return nil
}

func patchSveltosCluster(ctx context.Context, clusterNamespace, clusterName, shard string,
	labels map[string]string, renew bool, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	currentSveltosCluster := &libsveltosv1beta1.SveltosCluster{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName},
		currentSveltosCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating SveltosCluster %s/%s", clusterNamespace, clusterName))
			currentSveltosCluster.Namespace = clusterNamespace
			currentSveltosCluster.Name = clusterName
			currentSveltosCluster.Labels = labels
			currentSveltosCluster.Spec.KubeconfigKeyName = kubeconfig
			if renew {
				currentSveltosCluster.Spec.TokenRequestRenewalOption = &libsveltosv1beta1.TokenRequestRenewalOption{
					RenewTokenRequestInterval: metav1.Duration{Duration: 1 * time.Hour},
					TokenDuration:             metav1.Duration{Duration: 5 * time.Hour},
				}
			}
			if shard != "" {
				currentSveltosCluster.Annotations = map[string]string{
					shardingAnnotationKey: shard,
				}
			}

			return instance.CreateResource(ctx, currentSveltosCluster)
		}
		return err
	}

	logger.V(logs.LogDebug).Info("Updating SveltosCluster")
	currentSveltosCluster.Labels = labels
	currentSveltosCluster.Spec.KubeconfigKeyName = kubeconfig
	if shard != "" {
		currentSveltosCluster.Annotations = map[string]string{
			shardingAnnotationKey: shard,
		}
	} else if currentSveltosCluster.Annotations != nil {
		delete(currentSveltosCluster.Annotations, shardingAnnotationKey)
	}
	return instance.UpdateResource(ctx, currentSveltosCluster)
}

func patchSecret(ctx context.Context, clusterNamespace, secretName string, kubeconfigData []byte, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentSecret := &corev1.Secret{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, currentSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating Secret %s/%s", clusterNamespace, secretName))
			currentSecret.Namespace = clusterNamespace
			currentSecret.Name = secretName
			currentSecret.Data = map[string][]byte{kubeconfig: kubeconfigData}
			return instance.CreateResource(ctx, currentSecret)
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Secret %s/%s", clusterNamespace, secretName))
	currentSecret.Data = map[string][]byte{
		kubeconfig: kubeconfigData,
	}

	return instance.UpdateResource(ctx, currentSecret)
}

// RegisterCluster takes care of creating all necessary internal resources to import a cluster
func RegisterCluster(ctx context.Context, args []string, logger logr.Logger) error { //nolint: funlen,gocyclo,maintidx // command description
	doc := `Usage:
  sveltosctl register cluster [options] --namespace=<name> --cluster=<name> [--kubeconfig=<file>] [--fleet-cluster-context=<value>] [--pullmode]
                                [--workload-identity-provider=<name>] [--workload-identity-endpoint=<url>] [--workload-identity-ca-file=<file>]
                                [--aws-cluster-name=<name>] [--aws-role-arn=<arn>] [--aws-region=<region>]
                                [--gcp-project-id=<id>] [--gcp-cluster-name=<name>] [--gcp-location=<location>]
                                [--azure-tenant-id=<id>] [--azure-client-id=<id>] [--azure-subscription-id=<id>]
                                [--azure-resource-group=<group>] [--azure-cluster-name=<name>]
                                [--labels=<value>] [--shard=<key>] [--service-account-token] [--verbose]

     --namespace=<name>                  Specifies the namespace where Sveltos will create a resource (SveltosCluster) to represent
                                         the registered cluster.
     --cluster=<name>                    Defines a name for the registered cluster within Sveltos.
     --kubeconfig=<file>                 (Optional) Provides the path to a file containing the kubeconfig for the Kubernetes cluster
                                         you want to register.
                                         If you don't have a kubeconfig file yet, you can use the "sveltosctl generate kubeconfig"
                                         command.
                                         Be sure to point that command to the specific cluster you want to manage.
                                         This will help you create the necessary kubeconfig file before registering the cluster
                                         with Sveltos.
                                         Either --kubeconfig or --fleet-cluster-context must be provided.
     --fleet-cluster-context=<value>     (Optional) If your kubeconfig has multiple contexts:
                                         - One context points to the management cluster (default one)
                                         - Another context points to the cluster you actually want to manage;
                                         In this case, you can specify the context name with the --fleet-cluster-context flag.
                                         This tells the command to use the specific context to generate a Kubeconfig Sveltos
                                         can use and then create a SveltosCluster with it so you don't have to provide kubeconfig
                                         Either --kubeconfig or --fleet-cluster-context must be provided.
     --pullmode                          (Optional) this registers a cluster in pull mode. When enabled, the managed cluster will actively
                                         fetch its configurations from the management cluster, which is ideal for scenarios with
                                         firewall restrictions or when direct inbound access to the managed cluster is undesirable.
                                         This flag outputs the specialized YAML configuration that needs to be applied to the managed
                                         cluster to complete its setup.
     --workload-identity-provider=<name> (Optional) Cloud provider for workload identity authentication. One of: aws, gcp, azure.
                                         When set, --kubeconfig and --fleet-cluster-context are not used; Sveltos obtains
                                         short-lived credentials from the cloud provider at runtime.
                                         Mutually exclusive with --kubeconfig and --fleet-cluster-context.
     --workload-identity-endpoint=<url>  Required when --workload-identity-provider is set. The API server endpoint of the
                                         managed cluster (e.g. https://...).
     --workload-identity-ca-file=<file>  (Optional) Path to the CA certificate file for the managed cluster API server.
                                         When provided, sveltosctl creates a Secret named <cluster>-sveltos-ca in the
                                         cluster namespace and references it in the SveltosCluster.
     --aws-cluster-name=<name>           Required when --workload-identity-provider=aws. The EKS cluster name, embedded
                                         in the bearer token header sent to the EKS API server.
     --aws-role-arn=<arn>                (Optional) IAM role ARN to assume before generating the EKS bearer token.
                                         If omitted, the pod's own IRSA credentials are used directly.
     --aws-region=<region>               (Optional) AWS region of the EKS cluster. Defaults to the AWS_REGION environment
                                         variable injected by IRSA.
     --gcp-project-id=<id>               Required when --workload-identity-provider=gcp. The GCP project ID.
     --gcp-cluster-name=<name>           Required when --workload-identity-provider=gcp. The GKE cluster name.
     --gcp-location=<location>           Required when --workload-identity-provider=gcp. The GCP region or zone
                                         (e.g. us-central1-a).
     --azure-tenant-id=<id>              Required when --workload-identity-provider=azure. Azure AD tenant ID.
     --azure-client-id=<id>              Required when --workload-identity-provider=azure. Client ID of the managed
                                         identity or app registration federated with the management cluster service account.
     --azure-subscription-id=<id>        (Optional) Azure subscription containing the AKS cluster.
     --azure-resource-group=<group>      (Optional) Azure resource group containing the AKS cluster.
     --azure-cluster-name=<name>         (Optional) AKS cluster name.
     --labels=<key1=value1,key2=value2>  (Optional) This option allows you to specify labels for the SveltosCluster resource
                                         being created. The format for labels is <key1=value1,key2=value2>, where each key-value
                                         pair is separated by a comma (,) and the key and value are separated by an equal sign (=).
                                         You can define multiple labels by adding more key-value pairs separated by commas.
     --shard=<shard key>                 Optional. Assigns the cluster to a specific controller shard. This automatically adds the annotation
                                         sharding.projectsveltos.io/key: <value> to the SveltosCluster resource, ensuring the correct shard
                                         processes it.
     --service-account-token             (Optional) Use a non-expiring ServiceAccount token for management cluster registration.
                                         When enabled, Sveltos will automatically create the necessary ServiceAccount infrastructure
                                         (ServiceAccount, ClusterRole, and ClusterRoleBinding) in the managed cluster and
                                         generate a long-lived token by also creating a Secret of type kubernetes.io/service-account-token.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The register cluster command registers a cluster to be managed by Sveltos.
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

	var labels map[string]string
	if passedLabels := parsedArgs["--labels"]; passedLabels != nil {
		labels, err = stringToMap(passedLabels.(string))
		if err != nil {
			return err
		}
	}

	shard := ""
	if passedShard := parsedArgs["--shard"]; passedShard != nil {
		shard = passedShard.(string)
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	pullMode := parsedArgs["--pullmode"].(bool)

	renew := true

	satoken := parsedArgs["--service-account-token"].(bool)
	if satoken {
		renew = false
	}

	kubeconfig := ""
	if passedKubeconfig := parsedArgs["--kubeconfig"]; passedKubeconfig != nil {
		kubeconfig = passedKubeconfig.(string)
		// If Kubeconfig is passed, dont try to renew it
		renew = false
		satoken = false
	}

	fleetClusterContext := ""
	if passedContext := parsedArgs["--fleet-cluster-context"]; passedContext != nil {
		fleetClusterContext = passedContext.(string)
	}

	if pullMode {
		return onboardSveltosClusterInPullMode(ctx, namespace, cluster, shard, labels, logger)
	}

	wiProvider := ""
	if v := parsedArgs["--workload-identity-provider"]; v != nil {
		wiProvider = v.(string)
	}

	if wiProvider != "" {
		if kubeconfig != "" || fleetClusterContext != "" {
			return fmt.Errorf("--workload-identity-provider is mutually exclusive with --kubeconfig and --fleet-cluster-context")
		}
		wi, caFile, err := buildWorkloadIdentityConfig(parsedArgs)
		if err != nil {
			return err
		}
		return onboardSveltosClusterWithWorkloadIdentity(ctx, namespace, cluster, shard, wi, caFile, labels, logger)
	}

	if kubeconfig == "" && fleetClusterContext == "" {
		return fmt.Errorf("either kubeconfig or fleet-cluster-context must be specified")
	}

	data, err := getKubeconfigData(ctx, kubeconfig, fleetClusterContext, satoken, logger)
	if err != nil {
		return err
	}

	return onboardSveltosCluster(ctx, namespace, cluster, shard, data, labels, renew, logger)
}

func onboardSveltosClusterWithWorkloadIdentity(
	ctx context.Context,
	clusterNamespace, clusterName, shard string,
	wi *libsveltosv1beta1.WorkloadIdentityConfig,
	caFile string,
	labels map[string]string,
	logger logr.Logger,
) error {

	if caFile != "" {
		caData, err := os.ReadFile(caFile)
		if err != nil {
			return fmt.Errorf("failed to read CA file %s: %w", caFile, err)
		}
		caSecretName := clusterName + sveltosCASecretNamePostfix
		if err := patchCASecret(ctx, clusterNamespace, caSecretName, caData, logger); err != nil {
			return err
		}
		wi.CASecretRef = &corev1.LocalObjectReference{Name: caSecretName}
	}

	if err := patchSveltosClusterWithWorkloadIdentity(ctx, clusterNamespace, clusterName, shard, wi, labels, logger); err != nil {
		return err
	}

	//nolint: forbidigo // print success message
	fmt.Printf("cluster %s successfully registered/updated in namespace %s.", clusterName, clusterNamespace)
	return nil
}

func patchSveltosClusterWithWorkloadIdentity(
	ctx context.Context,
	clusterNamespace, clusterName, shard string,
	wi *libsveltosv1beta1.WorkloadIdentityConfig,
	labels map[string]string,
	logger logr.Logger,
) error {

	instance := utils.GetAccessInstance()

	current := &libsveltosv1beta1.SveltosCluster{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, current)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating SveltosCluster %s/%s", clusterNamespace, clusterName))
			current.Namespace = clusterNamespace
			current.Name = clusterName
			current.Labels = labels
			current.Spec.WorkloadIdentity = wi
			if shard != "" {
				current.Annotations = map[string]string{shardingAnnotationKey: shard}
			}
			return instance.CreateResource(ctx, current)
		}
		return err
	}

	logger.V(logs.LogDebug).Info("Updating SveltosCluster")
	current.Labels = labels
	current.Spec.WorkloadIdentity = wi
	current.Spec.KubeconfigKeyName = ""
	if shard != "" {
		if current.Annotations == nil {
			current.Annotations = map[string]string{}
		}
		current.Annotations[shardingAnnotationKey] = shard
	} else if current.Annotations != nil {
		delete(current.Annotations, shardingAnnotationKey)
	}
	return instance.UpdateResource(ctx, current)
}

func patchCASecret(ctx context.Context, clusterNamespace, secretName string, caData []byte, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	current := &corev1.Secret{}
	err := instance.GetResource(ctx, types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, current)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating CA Secret %s/%s", clusterNamespace, secretName))
			current.Namespace = clusterNamespace
			current.Name = secretName
			current.Data = map[string][]byte{caKey: caData}
			return instance.CreateResource(ctx, current)
		}
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating CA Secret %s/%s", clusterNamespace, secretName))
	current.Data = map[string][]byte{caKey: caData}
	return instance.UpdateResource(ctx, current)
}

func buildWorkloadIdentityConfig(parsedArgs map[string]interface{}) (*libsveltosv1beta1.WorkloadIdentityConfig, string, error) {
	endpoint := ""
	if v := parsedArgs["--workload-identity-endpoint"]; v != nil {
		endpoint = v.(string)
	}
	if endpoint == "" {
		return nil, "", fmt.Errorf("--workload-identity-endpoint is required when --workload-identity-provider is set")
	}

	caFile := ""
	if v := parsedArgs["--workload-identity-ca-file"]; v != nil {
		caFile = v.(string)
	}

	provider := parsedArgs["--workload-identity-provider"].(string)
	wi := &libsveltosv1beta1.WorkloadIdentityConfig{Endpoint: endpoint}

	switch provider {
	case "aws":
		if err := buildAWSWorkloadIdentity(parsedArgs, wi); err != nil {
			return nil, "", err
		}
	case "gcp":
		if err := buildGCPWorkloadIdentity(parsedArgs, wi); err != nil {
			return nil, "", err
		}
	case "azure":
		if err := buildAzureWorkloadIdentity(parsedArgs, wi); err != nil {
			return nil, "", err
		}
	default:
		return nil, "", fmt.Errorf("unknown workload identity provider %q: must be aws, gcp, or azure", provider)
	}

	return wi, caFile, nil
}

func buildAWSWorkloadIdentity(parsedArgs map[string]interface{}, wi *libsveltosv1beta1.WorkloadIdentityConfig) error {
	clusterName := ""
	if v := parsedArgs["--aws-cluster-name"]; v != nil {
		clusterName = v.(string)
	}
	if clusterName == "" {
		return fmt.Errorf("--aws-cluster-name is required when --workload-identity-provider=aws")
	}
	wi.Provider = libsveltosv1beta1.WorkloadIdentityProviderAWS
	wi.AWS = &libsveltosv1beta1.AWSWorkloadIdentityConfig{ClusterName: clusterName}
	if v := parsedArgs["--aws-role-arn"]; v != nil {
		wi.AWS.RoleARN = v.(string)
	}
	if v := parsedArgs["--aws-region"]; v != nil {
		wi.AWS.Region = v.(string)
	}
	return nil
}

func buildGCPWorkloadIdentity(parsedArgs map[string]interface{}, wi *libsveltosv1beta1.WorkloadIdentityConfig) error {
	projectID, clusterName, location := "", "", ""
	if v := parsedArgs["--gcp-project-id"]; v != nil {
		projectID = v.(string)
	}
	if v := parsedArgs["--gcp-cluster-name"]; v != nil {
		clusterName = v.(string)
	}
	if v := parsedArgs["--gcp-location"]; v != nil {
		location = v.(string)
	}
	if projectID == "" || clusterName == "" || location == "" {
		return fmt.Errorf("--gcp-project-id, --gcp-cluster-name, and --gcp-location are required when --workload-identity-provider=gcp")
	}
	wi.Provider = libsveltosv1beta1.WorkloadIdentityProviderGCP
	wi.GCP = &libsveltosv1beta1.GCPWorkloadIdentityConfig{
		ProjectID:   projectID,
		ClusterName: clusterName,
		Location:    location,
	}
	return nil
}

func buildAzureWorkloadIdentity(parsedArgs map[string]interface{}, wi *libsveltosv1beta1.WorkloadIdentityConfig) error {
	tenantID, clientID := "", ""
	if v := parsedArgs["--azure-tenant-id"]; v != nil {
		tenantID = v.(string)
	}
	if v := parsedArgs["--azure-client-id"]; v != nil {
		clientID = v.(string)
	}
	if tenantID == "" || clientID == "" {
		return fmt.Errorf("--azure-tenant-id and --azure-client-id are required when --workload-identity-provider=azure")
	}
	wi.Provider = libsveltosv1beta1.WorkloadIdentityProviderAzure
	wi.Azure = &libsveltosv1beta1.AzureWorkloadIdentityConfig{
		TenantID: tenantID,
		ClientID: clientID,
	}
	if v := parsedArgs["--azure-subscription-id"]; v != nil {
		wi.Azure.SubscriptionID = v.(string)
	}
	if v := parsedArgs["--azure-resource-group"]; v != nil {
		wi.Azure.ResourceGroup = v.(string)
	}
	if v := parsedArgs["--azure-cluster-name"]; v != nil {
		wi.Azure.ClusterName = v.(string)
	}
	return nil
}

func getKubeconfigData(ctx context.Context, kubeconfigFile, fleetClusterContext string,
	satoken bool, logger logr.Logger) ([]byte, error) {

	var data []byte
	if fleetClusterContext != "" {
		currentContext, err := getCurrentContext()
		if err != nil {
			return nil, err
		}
		kubeconfigData, err := createKubeconfig(ctx, fleetClusterContext, satoken, logger)
		if err != nil {
			return nil, err
		}
		err = switchCurrentContext(currentContext)
		if err != nil {
			return nil, err
		}
		data = []byte(kubeconfigData)
	} else {
		var err error
		data, err = os.ReadFile(kubeconfigFile)
		if err != nil {
			return nil, err
		}
	}

	return data, nil
}

func stringToMap(data string) (map[string]string, error) {
	const keyValueLength = 2
	result := make(map[string]string)
	for _, pair := range strings.Split(data, ",") {
		kv := strings.Split(pair, "=")
		if len(kv) != keyValueLength {
			return nil, fmt.Errorf("invalid key-value pair format: %s", pair)
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])
		result[key] = value
	}
	return result, nil
}

func createKubeconfig(ctx context.Context, fleetClusterContext string, satoken bool,
	logger logr.Logger) (string, error) {

	logger.V(logs.LogDebug).Info("Get current context")
	currentContext, err := getCurrentContext()
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("current context %s", currentContext))

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Switch context to %s", fleetClusterContext))
	err = switchCurrentContext(fleetClusterContext)
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("switched to context %s", fleetClusterContext))

	var remoteRestConfig *rest.Config
	remoteRestConfig, err = ctrl.GetConfig()
	if err != nil {
		return "", err
	}

	logger.V(logs.LogDebug).Info("Generate Kubeconfig")
	var data string
	data, err = generate.GenerateKubeconfigForServiceAccount(ctx, remoteRestConfig, generate.Projectsveltos,
		generate.Projectsveltos, 0, true, false, satoken, logger)
	if err != nil {
		return "", err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Reset context to %s", currentContext))
	err = switchCurrentContext(currentContext)
	if err != nil {
		return "", err
	}
	logger.V(logs.LogDebug).Info(fmt.Sprintf("switched to context %s", currentContext))

	return data, nil
}

func getCurrentContext() (string, error) {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.RawConfig()
	if err != nil {
		return "", err
	}

	return config.CurrentContext, nil
}

func switchCurrentContext(fleetClusterContext string) error {
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	config, err := kubeconfig.RawConfig()

	if err != nil {
		return err
	}

	for contextName := range config.Contexts {
		if contextName == fleetClusterContext {
			config.CurrentContext = fleetClusterContext
			err = clientcmd.ModifyConfig(clientcmd.NewDefaultPathOptions(), config, true)
			if err != nil {
				return fmt.Errorf("error ModifyConfig: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("error context %s not found", fleetClusterContext)
}
