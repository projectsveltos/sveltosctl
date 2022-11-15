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
	"k8s.io/klog/v2/klogr"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/snapshotter"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

func rollbackConfiguration(ctx context.Context,
	snapshotName, sample, passedNamespace, passedCluster, passedClusterProfile, passedClassifier string,
	logger logr.Logger) error {

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Getting Snapshot %s", snapshotName))

	// Get the directory containing the collected snapshots for Snapshot instance snapshotName
	instance := utils.GetAccessInstance()
	snapshotInstance := &utilsv1alpha1.Snapshot{}
	err := instance.GetResource(ctx, types.NamespacedName{Name: snapshotName}, snapshotInstance)
	if err != nil {
		return err
	}

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Getting snapshot folder for %s", sample))
	snapshotClient := snapshotter.GetClient()
	artifactFolder, err := snapshotClient.GetCollectedSnapshotFolder(snapshotInstance, logger)
	if err != nil {
		return err
	}

	folder := filepath.Join(*artifactFolder, sample)
	// Get the two directories containing the collected snaphosts
	_, err = os.Stat(folder)
	if os.IsNotExist(err) {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("Folder %s does not exist for snapshot instance: %s",
			folder, snapshotName))
		return err
	}

	return rollbackConfigurationToSnapshot(ctx, folder, passedNamespace, passedCluster, passedClusterProfile, passedClassifier, logger)
}

func rollbackConfigurationToSnapshot(ctx context.Context, folder, passedNamespace, passedCluster, passedClusterProfile, passedClassifier string,
	logger logr.Logger) error {

	err := getAndRollbackConfigMaps(ctx, folder, passedNamespace, logger)
	if err != nil {
		return err
	}

	err = getAndRollbackSecrets(ctx, folder, passedNamespace, logger)
	if err != nil {
		return err
	}

	err = getAndRollbackClusters(ctx, folder, passedNamespace, passedCluster, logger)
	if err != nil {
		return err
	}

	err = getAndRollbackClusterProfiles(ctx, folder, passedClusterProfile, logger)
	if err != nil {
		return err
	}

	err = getAndRollbackClassifiers(ctx, folder, passedClusterProfile, logger)
	if err != nil {
		return err
	}

	return nil
}

func getAndRollbackConfigMaps(ctx context.Context, folder, passedNamespace string, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	cmMap, err := snapshotClient.GetNamespacedResources(folder, "ConfigMap", logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ConfigMaps from folder %s", folder))
		return err
	}

	for ns := range cmMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback ConfigMaps in namespace %s", ns))
			err = rollbackConfigMaps(ctx, cmMap[ns], logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackSecrets(ctx context.Context, folder, passedNamespace string, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	secretMap, err := snapshotClient.GetNamespacedResources(folder, "Secret", logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect Secret from folder %s", folder))
		return err
	}

	for ns := range secretMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback ConfigMaps in namespace %s", ns))
			err = rollbackSecrets(ctx, secretMap[ns], logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackClusters(ctx context.Context, folder, passedNamespace, passedCluster string, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	clusterMap, err := snapshotClient.GetNamespacedResources(folder, "Cluster", logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect Cluster from folder %s", folder))
		return err
	}

	for ns := range clusterMap {
		if passedNamespace == "" || ns == passedNamespace {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback Clusters in namespace %s", ns))
			err = rollbackClusters(ctx, clusterMap[ns], passedCluster, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackClusterProfiles(ctx context.Context, folder, passedClusterProfile string, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	clusterProfiles, err := snapshotClient.GetClusterResources(folder, configv1alpha1.ClusterProfileKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterProfile from folder %s", folder))
		return err
	}

	for i := range clusterProfiles {
		cp := clusterProfiles[i]
		if passedClusterProfile == "" || cp.GetName() == passedClusterProfile {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback ClusterProfile %s", cp.GetName()))
			err = rollbackClusterProfile(ctx, cp, logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func getAndRollbackClassifiers(ctx context.Context, folder, passedClassifier string, logger logr.Logger) error {
	snapshotClient := snapshotter.GetClient()
	classifiers, err := snapshotClient.GetClassifierResources(folder, libsveltosv1alpha1.ClassifierKind, logger)
	if err != nil {
		logger.V(logs.LogVerbose).Info(fmt.Sprintf("failed to collect ClusterProfile from folder %s", folder))
		return err
	}

	for i := range classifiers {
		cl := classifiers[i]
		if passedClassifier == "" || cl.GetName() == passedClassifier {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback Classifier %s", cl.GetName()))
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
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Creating ConfigMap %s/%s",
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

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Updating ConfigMap %s/%s",
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
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Creating Secret %s/%s",
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

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Updating Secret %s/%s",
		resource.GetNamespace(), resource.GetName()))
	return instance.UpdateResource(ctx, currentSecret)
}

func rollbackClusters(ctx context.Context, resources []*unstructured.Unstructured, passedCluster string,
	logger logr.Logger) error {

	for i := range resources {
		if passedCluster == "" || resources[i].GetName() == passedCluster {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("rollback Cluster %s", resources[i].GetName()))
			err := rollbackCluster(ctx, resources[i], logger)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// rollbackCluster does not nothing if Cluster currently does not exist.
// If Cluster currently exists, then it updates Cluster.Labels
func rollbackCluster(ctx context.Context, resource *unstructured.Unstructured, logger logr.Logger) error {
	instance := utils.GetAccessInstance()

	currentCluster := &clusterv1.Cluster{}
	err := instance.GetResource(ctx,
		types.NamespacedName{Namespace: resource.GetNamespace(), Name: resource.GetName()},
		currentCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Cluster %s does not exist anymore.",
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

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Updating Cluster %s",
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
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Creating ClusterProfile %s",
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

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Updating ClusterProfile %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentClusterProfile)
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
			logger.V(logs.LogVerbose).Info(fmt.Sprintf("Creating Classifier %s",
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

	logger.V(logs.LogVerbose).Info(fmt.Sprintf("Updating Classifier %s",
		resource.GetName()))
	return instance.UpdateResource(ctx, currentClassifier)
}

// Rollback system to any previous configuration snapshot
func Rollback(ctx context.Context, args []string, logger logr.Logger) error {
	//nolint: lll // command syntax
	doc := `Usage:
	sveltosctl snapshot rollback [options] --snapshot=<name> --sample=<name> [--namespace=<name>] [--clusterprofile=<name>] [--cluster=<name>] [--classifier=<name>] [--verbose]

     --snapshot=<name>       Name of the Snapshot instance
     --sample=<name>         Name of the directory containing this sample.
                             Use sveltosctl snapshot list to see all collected snapshosts.
     --namespace=<name>      Rollbacks only ConfigMaps/Secrets and Cluster labels in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Rollback only clusters with this name.
                             If not specified all clusters are updated.
     --clusterprofile=<name> Rollback only clusterprofile with this name.
                             If not specified all clusterprofiles are updated.
     --classifier=<name>     Rollback only classifier with this name.
                             If not specified all classifiers are updated.							 

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.  

Description:
  The snapshot rollback allows to rollback system to any previous configuration snapshot.
  Following objects will be rolled back:
  - ClusterProfiles, Spec sections
  - ConfigMaps/Secrets referenced by at least one ClusterProfile at the time snapshot was taken. 
  If, at the time the rollback happens, such resources do not exist, those will be recreated.
  If such resources exist, Data/BinaryData for ConfigMaps and Data/StringData for Secrets will be updated.
  - Clusters, only labels will be updated.
`

	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogVerbose).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		err = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogVerbose))
		if err != nil {
			return err
		}
	}

	logger = klogr.New()

	snapshostName := parsedArgs["--snapshot"].(string)
	sample := parsedArgs["--sample"].(string)

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	clusterProfile := ""
	if passedClusterProfile := parsedArgs["--clusterProfile"]; passedClusterProfile != nil {
		clusterProfile = passedClusterProfile.(string)
	}

	classifier := ""
	if passedClassifier := parsedArgs["--classifier"]; passedClassifier != nil {
		classifier = passedClassifier.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	return rollbackConfiguration(ctx, snapshostName, sample, namespace, cluster, clusterProfile, classifier, logger)
}
