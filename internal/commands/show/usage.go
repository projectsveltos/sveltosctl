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

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var (
	// resourceKind indentifies the type of resource (ClusterProfile, ConfigMap, Secret)
	// resourceNamespace and resourceName is the kubernetes resource namespace/name
	// clusters is the list of clusters where resource content is deployed
	genUsageRow = func(resourceKind, resourceNamespace, resourceName string, clusters []string,
	) []string {
		return []string{
			resourceKind,
			resourceNamespace,
			resourceName,
			strings.Join(clusters, "\n"),
		}
	}
)

func showUsage(ctx context.Context, kind, passedNamespace, passedName string, logger logr.Logger) error {
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"RESOURCE KIND", "RESOURCE NAMESPACE", "RESOURCE NAME", "CLUSTERS"})

	if kind == "" || kind == configv1alpha1.ClusterProfileKind {
		if err := showUsageForClusterProfiles(ctx, passedName, table, logger); err != nil {
			return err
		}
	}
	if kind == "" || kind == string(libsveltosv1alpha1.ConfigMapReferencedResourceKind) {
		if err := showUsageForConfigMaps(ctx, passedNamespace, passedName, table, logger); err != nil {
			return err
		}
	}
	if kind == "" || kind == string(libsveltosv1alpha1.SecretReferencedResourceKind) {
		if err := showUsageForSecrets(ctx, passedNamespace, passedName, table, logger); err != nil {
			return err
		}
	}

	table.Render()

	return nil
}

func showUsageForClusterProfiles(ctx context.Context, passedName string, table *tablewriter.Table, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	cps, err := instance.ListClusterProfiles(ctx, logger)
	if err != nil {
		return err
	}

	for i := range cps.Items {
		cp := &cps.Items[i]
		if passedName == "" || cp.Name == passedName {
			showUsageForClusterProfile(cp, table, logger)
		}
	}

	return nil
}

func getMatchingClusters(clusterProfile *configv1alpha1.ClusterProfile) []string {
	clusters := make([]string, len(clusterProfile.Status.MatchingClusterRefs))
	for i := range clusterProfile.Status.MatchingClusterRefs {
		c := &clusterProfile.Status.MatchingClusterRefs[i]
		clusters[i] = fmt.Sprintf("%s/%s", c.Namespace, c.Name)
	}
	return clusters
}

func showUsageForClusterProfile(clusterProfile *configv1alpha1.ClusterProfile, table *tablewriter.Table, logger logr.Logger) {
	logger.V(logs.LogDebug).Info(fmt.Sprintf("Considering ClusterProfile %s", clusterProfile.Name))

	clusters := getMatchingClusters(clusterProfile)

	table.Append(genUsageRow(configv1alpha1.ClusterProfileKind, "", clusterProfile.Name, clusters))
}

func showUsageForConfigMaps(ctx context.Context, passedNamespace, passedName string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	cps, err := instance.ListClusterProfiles(ctx, logger)
	if err != nil {
		return err
	}

	result := make(map[libsveltosv1alpha1.PolicyRef][]string)

	for i := range cps.Items {
		cp := &cps.Items[i]
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Collect referenced ConfigMaps from ClusterProfile %s", cp.Name))
		getConfigMaps(passedNamespace, passedName, cp, result, logger)
	}

	for pr := range result {
		table.Append(genUsageRow(string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
			pr.Namespace, pr.Name, result[pr]))
	}

	return nil
}

func showUsageForSecrets(ctx context.Context, passedNamespace, passedName string,
	table *tablewriter.Table, logger logr.Logger) error {

	instance := utils.GetAccessInstance()

	cps, err := instance.ListClusterProfiles(ctx, logger)
	if err != nil {
		return err
	}

	result := make(map[libsveltosv1alpha1.PolicyRef][]string)

	for i := range cps.Items {
		cp := &cps.Items[i]
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Collect referenced Secret from ClusterProfile %s", cp.Name))
		getSecrets(passedNamespace, passedName, cp, result, logger)
	}

	for pr := range result {
		table.Append(genUsageRow(string(libsveltosv1alpha1.SecretReferencedResourceKind),
			pr.Namespace, pr.Name, result[pr]))
	}

	return nil
}

func getConfigMaps(passedNamespace, passedName string, clusterProfile *configv1alpha1.ClusterProfile,
	result map[libsveltosv1alpha1.PolicyRef][]string, logger logr.Logger) {

	configMaps := make([]libsveltosv1alpha1.PolicyRef, 0)
	for i := range clusterProfile.Spec.PolicyRefs {
		pr := &clusterProfile.Spec.PolicyRefs[i]
		if pr.Kind == string(libsveltosv1alpha1.ConfigMapReferencedResourceKind) {
			if shouldAddPolicyRef(passedNamespace, passedName, pr) {
				logger.V(logs.LogDebug).Info(fmt.Sprintf("considering reference configMap %s/%s",
					pr.Namespace, pr.Name))
				configMaps = append(configMaps, *pr)
			}
		}
	}

	clusters := getMatchingClusters(clusterProfile)

	for i := range configMaps {
		cm := &configMaps[i]
		if _, ok := result[*cm]; !ok {
			result[*cm] = make([]string, 0)
		}
		result[*cm] = append(result[*cm], clusters...)
	}
}

func getSecrets(passedNamespace, passedName string, clusterProfile *configv1alpha1.ClusterProfile,
	result map[libsveltosv1alpha1.PolicyRef][]string, logger logr.Logger) {

	secrets := make([]libsveltosv1alpha1.PolicyRef, 0)
	for i := range clusterProfile.Spec.PolicyRefs {
		pr := &clusterProfile.Spec.PolicyRefs[i]
		if pr.Kind == string(libsveltosv1alpha1.SecretReferencedResourceKind) {
			if shouldAddPolicyRef(passedNamespace, passedName, pr) {
				logger.V(logs.LogDebug).Info(fmt.Sprintf("considering reference secret %s/%s",
					pr.Namespace, pr.Name))
				secrets = append(secrets, *pr)
			}
		}
	}

	clusters := getMatchingClusters(clusterProfile)

	for i := range secrets {
		secret := &secrets[i]
		if _, ok := result[*secret]; !ok {
			result[*secret] = make([]string, 0)
		}
		result[*secret] = append(result[*secret], clusters...)
	}
}

func shouldAddPolicyRef(passedNamespace, passedName string, pr *libsveltosv1alpha1.PolicyRef) bool {
	if passedNamespace != "" &&
		pr.Namespace != passedNamespace {

		return false
	}

	if passedName != "" &&
		pr.Name != passedName {

		return false
	}

	return true
}

// Usage displays CAPI cluster where policies (ClusterProfiles and referenced ConfigMaps/Secrets) are deployed
func Usage(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show usage [options] [--kind=<name>] [--namespace=<resourceNamespace>] [--name=<resourceName>] [--verbose]

     --kind=<name>                    Show usage information for resources of this Kind only.
                                      If not specified, ClusterProfile and referenced ConfigMap and Secret are considered.
     --namespace=<resourceNamespace>  Show usage information for resources in this namespace only.
                                      If not specified all namespaces are considered.
     --name=<resourceName>            Show usage information for resources with this name only.
                                      If not specified all ClusterProfiles/ConfigMaps/Secrets are considered.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The show usage command display usage information:
  - for each ClusterProfile lists all CAPI clusters currently matching;
  - for each ConfigMap/Secret referenced by at least one ClusterProfile, lists all CAPI clusters where content of such resource is currently deployed.
`
	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogDebug).Error(err, "failed to parse args")
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

	name := ""
	if passedName := parsedArgs["--name"]; passedName != nil {
		name = passedName.(string)
	}

	kind := ""
	if passedKind := parsedArgs["--kind"]; passedKind != nil {
		kind = passedKind.(string)
		if kind != configv1alpha1.ClusterProfileKind &&
			kind != string(libsveltosv1alpha1.ConfigMapReferencedResourceKind) &&
			kind != string(libsveltosv1alpha1.SecretReferencedResourceKind) {

			return fmt.Errorf("possible values for kind are: %s, %s, %s",
				configv1alpha1.ClusterProfileKind,
				string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
				string(libsveltosv1alpha1.SecretReferencedResourceKind),
			)
		}
	}

	return showUsage(ctx, kind, namespace, name, logger)
}
