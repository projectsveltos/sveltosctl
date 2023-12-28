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

package snapshot

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1alpha1"
	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func rollbackConfiguration(ctx context.Context,
	snapshotName, sample, passedNamespace, passedCluster, passedProfile,
	passedClassifier, passedRoleRequest, passedAddonCompliance string,
	logger logr.Logger) error {

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Getting Snapshot %s", snapshotName))

	// Get the directory containing the collected snapshots for Snapshot instance snapshotName
	instance := utils.GetAccessInstance()
	snapshotInstance := &utilsv1alpha1.Snapshot{}
	err := instance.GetResource(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Getting snapshot folder for %s", sample))
	snapshotClient := collector.GetClient()
	artifactFolder, err := snapshotClient.GetFolder(snapshotInstance.Spec.Storage, snapshotInstance.Name,
		collector.Snapshot, logger)
	if err != nil {
		return err
	}

	folder := filepath.Join(*artifactFolder, sample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(folder)
	if os.IsNotExist(err) {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			folder, snapshotName))
		return err
	}

	return rollbackConfigurationToSnapshot(ctx, folder, passedNamespace, passedCluster, passedProfile,
		passedClassifier, passedRoleRequest, passedAddonCompliance, logger)
}

func rollbackConfigurationToSnapshot(ctx context.Context, folder, passedNamespace, passedCluster,
	passedProfile, passedClassifier, passedRoleRequest, passedAddonCompliance string,
	logger logr.Logger) error {

	logger.V(logs.LogDebug).Info("roll back configuration: configmaps")
	err := getAndRollbackConfigMaps(ctx, folder, passedNamespace, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: secrets")
	err = getAndRollbackSecrets(ctx, folder, passedNamespace, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: clusters")
	err = getAndRollbackClusters(ctx, folder, passedNamespace, passedCluster, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: profile")
	err = getAndRollbackProfiles(ctx, folder, passedNamespace, passedProfile, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: classifiers")
	err = getAndRollbackClassifiers(ctx, folder, passedClassifier, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: rolerequests")
	err = getAndRollbackRoleRequests(ctx, folder, passedRoleRequest, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("roll back configuration: addoncompliances")
	err = getAndRollbackAddonCompliances(ctx, folder, passedAddonCompliance, logger)
	if err != nil {
		return err
	}

	logger.V(logs.LogDebug).Info("rolled back configuration")

	return nil
}

func getAndRollbackConfigMaps(ctx context.Context, folder, passedNamespace string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	cmMap, err := snapshotClient.GetNamespacedResources(folder, "ConfigMap", logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect ConfigMaps from folder %s", folder))
		return err
	}

	for ns := range cmMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback ConfigMaps in namespace %s", ns))
			err = rollbackConfigMaps(ctx, cmMap[ns], logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackSecrets(ctx context.Context, folder, passedNamespace string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	secretMap, err := snapshotClient.GetNamespacedResources(folder, "Secret", logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect Secret from folder %s", folder))
		return err
	}

	for ns := range secretMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback ConfigMaps in namespace %s", ns))
			err = rollbackSecrets(ctx, secretMap[ns], logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackClusters(ctx context.Context, folder, passedNamespace, passedCluster string, logger logr.Logger) error {
	if err := getAndRollbackCAPIClusters(ctx, folder, passedNamespace, passedCluster, logger); err != nil {
		return err
	}

	if err := getAndRollbackSveltosClusters(ctx, folder, passedNamespace, passedCluster, logger); err != nil {
		return err
	}

	return nil
}

func getAndRollbackCAPIClusters(ctx context.Context, folder, passedNamespace, passedCluster string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	clusterMap, err := snapshotClient.GetNamespacedResources(folder, "Cluster", logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect Cluster from folder %s", folder))
		return err
	}

	for ns := range clusterMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Clusters in namespace %s", ns))
			err = rollbackClusters(ctx, clusterMap[ns], passedCluster, libsveltosv1alpha1.ClusterTypeCapi, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackSveltosClusters(ctx context.Context, folder, passedNamespace, passedCluster string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	clusterMap, err := snapshotClient.GetNamespacedResources(folder, libsveltosv1alpha1.SveltosClusterKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect SveltosCluster from folder %s", folder))
		return err
	}

	for ns := range clusterMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Clusters in namespace %s", ns))
			err = rollbackClusters(ctx, clusterMap[ns], passedCluster, libsveltosv1alpha1.ClusterTypeSveltos, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackProfiles(ctx context.Context, folder, passedNamespace, passedProfile string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	clusterProfiles, err := snapshotClient.GetClusterResources(folder, configv1alpha1.ClusterProfileKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect ClusterProfile from folder %s", folder))
		return err
	}

	for i := range clusterProfiles {
		cp := clusterProfiles[i]
		if passedProfile == "" || cp.GetName() == fmt.Sprintf("ClusterProfile/%s", passedProfile) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback ClusterProfile %s", cp.GetName()))
			err = rollbackClusterProfile(ctx, cp, logger)
			if err != nil {
				return err
			}
		}
	}

	profiles, err := snapshotClient.GetNamespacedResources(folder, configv1alpha1.ProfileKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect Profile from folder %s", folder))
		return err
	}

	for ns := range profiles {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Profiles in namespace %s", ns))
			err = rollbackProfiles(ctx, profiles[ns], passedProfile, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func rollbackProfiles(ctx context.Context, resources []*unstructured.Unstructured,
	passedProfile string, logger logr.Logger) error {

	for i := range resources {
		p := resources[i]
		if passedProfile == "" || p.GetName() == fmt.Sprintf("Profile/%s", passedProfile) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Profile %s", p.GetName()))
			err := rollbackProfile(ctx, p, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackClassifiers(ctx context.Context, folder, passedClassifier string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	classifiers, err := snapshotClient.GetClusterResources(folder, libsveltosv1alpha1.ClassifierKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect Classifiers from folder %s", folder))
		return err
	}

	for i := range classifiers {
		cl := classifiers[i]
		if passedClassifier == "" || cl.GetName() == passedClassifier {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Classifier %s", cl.GetName()))
			err = rollbackClassifier(ctx, cl, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackAddonCompliances(ctx context.Context, folder, passedAddonCompliance string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	addonCompliances, err := snapshotClient.GetClusterResources(folder, libsveltosv1alpha1.AddonComplianceKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect AddonCompliances from folder %s", folder))
		return err
	}

	for i := range addonCompliances {
		ac := addonCompliances[i]
		if passedAddonCompliance == "" || ac.GetName() == passedAddonCompliance {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback RoleRequest %s", ac.GetName()))
			err = rollbackClassifier(ctx, ac, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackRoleRequests(ctx context.Context, folder, passedRoleRequest string, logger logr.Logger) error {
	snapshotClient := collector.GetClient()
	roleRequests, err := snapshotClient.GetClusterResources(folder, libsveltosv1alpha1.RoleRequestKind, logger)
	if err != nil {
		logger.V(logs.LogDebug).Info(fmt.Sprintf("failed to collect RoleRequests from folder %s", folder))
		return err
	}

	for i := range roleRequests {
		cl := roleRequests[i]
		if passedRoleRequest == "" || cl.GetName() == passedRoleRequest {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback RoleRequest %s", cl.GetName()))
			err = rollbackClassifier(ctx, cl, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func rollbackConfigMaps(ctx context.Context, resources []*unstructured.Unstructured, logger logr.Logger) error {
	for i := range resources {
		err := rollbackConfigMap(ctx, resources[i], logger)
		if err != nil {
			return err
		}
	}

	return nil
}

// rollbackConfigMap does following:
// - if ConfigMap currently does not exist, recreates it
// - if ConfigMap does exist, updates it Data/BinaryData
func rollbackConfigMap(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentConfigMap := &corev1.ConfigMap{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentConfigMap)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating ConfigMap %s/%s",
				resource.GetNamespace(), resource.GetName()))
			return instance.CreateResource(ctx, resource)
		}
		return err
	}

	passedConfigMap := &corev1.ConfigMap{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedConfigMap)
	if err != nil {
		return err
	}

	currentConfigMap.Data = passedConfigMap.Data
	currentConfigMap.BinaryData = passedConfigMap.BinaryData

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating ConfigMap %s/%s",
		resource.GetNamespace(), resource.GetName()))
	return instance.UpdateResource(ctx, currentConfigMap)
}

func rollbackSecrets(ctx context.Context, resources []*unstructured.Unstructured, logger logr.Logger) error {
	for i := range resources {
		err := rollbackSecret(ctx, resources[i], logger)
		if err != nil {
			return err
		}
	}

	return nil
}

// rollbackSecret does following:
// - if Secret currently does not exist, recreates it
// - if Secret does exist, updates it Data/StringData
func rollbackSecret(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentSecret := &corev1.Secret{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentSecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating Secret %s/%s",
				resource.GetNamespace(), resource.GetName()))
			return instance.CreateResource(ctx, resource)
		}
		return err
	}

	passedSecret := &corev1.Secret{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedSecret)
	if err != nil {
		return err
	}

	currentSecret.Data = passedSecret.Data
	currentSecret.StringData = passedSecret.StringData

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Secret %s/%s",
		resource.GetNamespace(), resource.GetName()))
	return instance.UpdateResource(ctx, currentSecret)
}

func rollbackClusters(ctx context.Context, resources []*unstructured.Unstructured, passedCluster string,
	clusterType libsveltosv1alpha1.ClusterType, logger logr.Logger) error {

	for i := range resources {
		if passedCluster == "" || resources[i].GetName() == passedCluster {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("rollback Cluster %s", resources[i].GetName()))
			err := rollbackCluster(ctx, resources[i], clusterType, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// rollbackCluster does not nothing if Cluster currently does not exist.
// If Cluster currently exists, then it updates Cluster.Labels
func rollbackCluster(ctx context.Context, resource *unstructured.Unstructured,
	clusterType libsveltosv1alpha1.ClusterType, logger logr.Logger) error {

	if clusterType == libsveltosv1alpha1.ClusterTypeCapi {
		return rollbackCAPICluster(ctx, resource, logger)
	}

	return rollbackSveltosCluster(ctx, resource, logger)
}

func rollbackCAPICluster(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentCluster := &clusterv1.Cluster{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Cluster %s does not exist anymore.",
				resource.GetName()))
			return nil
		}
		return err
	}

	passedCluster := &clusterv1.Cluster{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedCluster)
	if err != nil {
		return err
	}

	currentCluster.Labels = passedCluster.Labels

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Cluster %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentCluster)
}

func rollbackSveltosCluster(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentCluster := &libsveltosv1alpha1.SveltosCluster{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Cluster %s does not exist anymore.",
				resource.GetName()))
			return nil
		}
		return err
	}

	passedCluster := &libsveltosv1alpha1.SveltosCluster{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedCluster)
	if err != nil {
		return err
	}

	currentCluster.Labels = passedCluster.Labels

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Cluster %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentCluster)
}

// rollbackClusterProfile does following:
// - if ClusterProfile currently does not exist, recreates it
// - if ClusterProfile does exist, updates it Spec section
func rollbackClusterProfile(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentClusterProfile := &configv1alpha1.ClusterProfile{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Name: resource.GetName()}, currentClusterProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating ClusterProfile %s",
				resource.GetName()))
			return instance.CreateResource(ctx, resource)
		}
		return err
	}

	passedClusterProfile := &configv1alpha1.ClusterProfile{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedClusterProfile)
	if err != nil {
		return err
	}

	currentClusterProfile.Spec = passedClusterProfile.Spec

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating ClusterProfile %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentClusterProfile)
}

// rollbackProfile does following:
// - if Profile currently does not exist, recreates it
// - if Profile does exist, updates it Spec section
func rollbackProfile(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentProfile := &configv1alpha1.Profile{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentProfile)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating Profile %s/%s",
				resource.GetNamespace(), resource.GetName()))
			return instance.CreateResource(ctx, resource)
		}
		return err
	}

	passedProfile := &configv1alpha1.Profile{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedProfile)
	if err != nil {
		return err
	}

	currentProfile.Spec = passedProfile.Spec

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Profile %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentProfile)
}

// rollbackClassifier does following:
// - if Classifier currently does not exist, recreates it
// - if Classifier does exist, updates it Spec section
func rollbackClassifier(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentClassifier := &libsveltosv1alpha1.Classifier{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Name: resource.GetName()}, currentClassifier)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogDebug).Info(fmt.Sprintf("Creating Classifier %s",
				resource.GetName()))
			return instance.CreateResource(ctx, resource)
		}
		return err
	}

	passedClassifier := &libsveltosv1alpha1.Classifier{}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(resource.UnstructuredContent(), passedClassifier)
	if err != nil {
		return err
	}

	currentClassifier.Spec = passedClassifier.Spec

	logger.V(logs.LogDebug).Info(fmt.Sprintf("Updating Classifier %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentClassifier)
}

// Rollback system to any previous configuration snapshot
func Rollback(ctx context.Context, args []string, logger logr.Logger) error {
	//nolint: lll // command syntax
	doc := `Usage:
	sveltosctl snapshot rollback [options] --snapshot=<name> --sample=<name> [--namespace=<name>] [--profile=<name>] [--cluster=<name>] [--classifier=<name>] [--rolerequest=<name>] [--addoncompliance=<name>] [--verbose]

     --snapshot=<name>       Name of the Snapshot instance
     --sample=<name>         Name of the directory containing this sample.
                             Use sveltosctl snapshot list to see all collected snapshosts.
     --namespace=<name>      Rollbacks only ConfigMaps/Secrets and Cluster labels in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Rollback only clusters with this name.
                             If not specified all clusters are updated.
     --profile=<kind/name>   Rollback only clusterprofile/profile with this name.
                             If not specified all clusterprofiles are updated.
     --classifier=<name>     Rollback only classifier with this name.
                             If not specified all classifiers are updated.
     --rolerequest=<name>    Rollback only roleRequest with this name.
                             If not specified all roleRequests are updated.
     --addonconstaint=<name> Rollback only addoncompliance with this name.
                             If not specified all addoncompliances are updated.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The snapshot rollback allows to rollback system to any previous configuration snapshot.
  Following objects will be rolled back:
  - ClusterProfiles, Labels and Spec sections
  - RoleRequest, Spec section
  - ConfigMaps/Secrets referenced by at least one ClusterProfile/RoleRequest at the time snapshot was taken.
  - Classifiers
  - AddonCompliances
  If, at the time the rollback happens, such resources do not exist, those will be recreated.
  If such resources exist, Data/BinaryData for ConfigMaps and Data/StringData for Secrets will be updated.
  - Clusters, only labels will be updated.
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

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug))
		if err != nil {
			return err
		}
	}

	logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))

	snapshostName := parsedArgs["--snapshot"].(string)
	sample := parsedArgs["--sample"].(string)

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	profile := ""
	if passedProfile := parsedArgs["--profile"]; passedProfile != nil {
		profile = passedProfile.(string)
	}

	classifier := ""
	if passedClassifier := parsedArgs["--classifier"]; passedClassifier != nil {
		classifier = passedClassifier.(string)
	}

	roleRequest := ""
	if passedRoleRequest := parsedArgs["--rolerequest"]; passedRoleRequest != nil {
		roleRequest = passedRoleRequest.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	addonCompliance := ""
	if passedAddonCompliance := parsedArgs["--addoncompliance"]; passedAddonCompliance != nil {
		addonCompliance = passedAddonCompliance.(string)
	}

	return rollbackConfiguration(ctx, snapshostName, sample, namespace, cluster, profile,
		classifier, roleRequest, addonCompliance, logger)
}
