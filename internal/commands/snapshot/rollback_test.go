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

package snapshot_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	kubectlscheme "k8s.io/kubectl/pkg/scheme"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/collector"
	"github.com/projectsveltos/sveltosctl/internal/commands/snapshot"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	configMapTemplate = `apiVersion: v1
kind: ConfigMap
metadata:
  namespace: %s
  name: %s 
data:
  game.properties: |
    enemies=aliens
    lives=3
    enemies.cheat=true
    enemies.cheat.level=noGoodRotten
    secret.code.passphrase=UUDDLRLRBABAS
    secret.code.allowed=true
    secret.code.lives=30    
  ui.properties: |
    color.good=purple
    color.bad=yellow
    allow.textmode=true
    how.nice.to.look=fairlyNice`

	//nolint: gosec // simply a test
	secretTemplate = `apiVersion: v1
data:
  username: YWRtaW4=
  password: MWYyZDFlMmU2N2Rm
kind: Secret
metadata:
  namespace: %s
  name: %s
type: Opaque`

	clusterProfileTemplate = `apiVersion: config.projectsveltos.io/v1beta1
kind: ClusterProfile
metadata:
  name: %s
spec:
  clusterSelector:
    matchLabels:
      env: fv
  policyRefs:
  - kind: ConfigMap
    name: featurei9nyv583mu
    namespace: sli0l4jkq2
  - kind: Secret
    name: featureamhrhgrzo7
    namespace: sli0l4jkq2
  syncMode: Continuous`

	clusterTemplate = `apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  namespace: %s
  name: %s
  labels:
    env: production
    dep: eng`

	//nolint: gosec // test only
	configMapWithPolicy = `data:
  kyverno.yaml: |
    apiVersion: kyverno.io/v1
    kind: ClusterPolicy
    metadata:
      name: no-gateway
      annotations:
        policies.kyverno.io/title: Block Create,Update,Delete of Gateway instances
        policies.kyverno.io/severity: medium
        policies.kyverno.io/subject: Gateway
        policies.kyverno.io/description: >-
          Management cluster admin controls Gateway's configurations.
    spec:
      validationFailureAction: enforce
      background: false
      rules:
      - name: block-gateway-updates
        match:
          any:
          - resources:
              kinds:
              - Gateway
        exclude:
          any:
          - clusterRoles:
            - cluster-admin
        validate:
          message: "Gateway's configurations is managed by management cluster admin."
          deny:
            conditions:
              - key: "{{request.operation}}"
                operator: In
                value:
                - CREATE
                - DELETE
                - UPDATE
metadata:
  annotations:
    manager: kubectl
    operation: Update
  namespace: %s
  name: %s
apiVersion: v1
kind: ConfigMap`
)

var _ = Describe("Snapshot Rollback", func() {
	It("rollbackConfigMaps rollbacks configMaps", func() {
		name := randomString()
		namespace := randomString()
		configMap := getConfigMap(namespace, name)

		initObjects := []client.Object{configMap}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		instance := utils.GetAccessInstance()

		currentConfigMap := &corev1.ConfigMap{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)).To(Succeed())

		// Save current configMap Data
		originalData := make(map[string]string)
		for k := range currentConfigMap.Data {
			originalData[k] = currentConfigMap.Data[k]
		}

		updateConfigMapData(currentConfigMap)

		// Rollback
		Expect(snapshot.RollbackConfigMaps(context.TODO(), []*unstructured.Unstructured{configMap},
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)).To(Succeed())
		Expect(reflect.DeepEqual(currentConfigMap.Data, originalData)).To(BeTrue())
	})

	It("rollbackSecrets rollbacks secrets", func() {
		name := randomString()
		namespace := randomString()
		secret := getSecret(namespace, name)

		initObjects := []client.Object{secret}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		instance := utils.GetAccessInstance()

		currentSecret := &corev1.Secret{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentSecret)).To(Succeed())

		// Save current secret Data
		originalData := make(map[string][]byte)
		for k := range currentSecret.Data {
			originalData[k] = currentSecret.Data[k]
		}

		updateSecretData(currentSecret)

		// Rollback
		Expect(snapshot.RollbackSecrets(context.TODO(), []*unstructured.Unstructured{secret},
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentSecret)).To(Succeed())
		Expect(reflect.DeepEqual(currentSecret.Data, originalData)).To(BeTrue())
	})

	It("rollbackClusterProfile rollbacks clusterProfile", func() {
		name := randomString()
		cp := getClusterProfile(name)

		initObjects := []client.Object{cp}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		instance := utils.GetAccessInstance()

		currentCP := &configv1alpha1.ClusterProfile{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Name: name}, currentCP)).To(Succeed())

		// Save current configMap Data
		originalSpec := currentCP.Spec

		updateClusterProfileSpec(currentCP)

		// Rollback
		Expect(snapshot.RollbackClusterProfile(context.TODO(), cp,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Name: name}, currentCP)).To(Succeed())
		Expect(reflect.DeepEqual(currentCP.Spec, originalSpec)).To(BeTrue())
	})

	It("rollbackClusters rollbacks clusters", func() {
		name := randomString()
		namespace := randomString()
		cluster := getCluster(namespace, name)

		initObjects := []client.Object{cluster}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		instance := utils.GetAccessInstance()

		currentCluster := &clusterv1.Cluster{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentCluster)).To(Succeed())

		// Save current cluster labels
		originalLabels := make(map[string]string)
		for k := range currentCluster.Labels {
			originalLabels[k] = currentCluster.Labels[k]
		}

		updateClusterLabels(currentCluster)

		// Rollback
		Expect(snapshot.RollbackClusters(context.TODO(), []*unstructured.Unstructured{cluster}, "",
			libsveltosv1beta1.ClusterTypeCapi,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentCluster)).To(Succeed())
		Expect(reflect.DeepEqual(currentCluster.Labels, originalLabels)).To(BeTrue())
	})

	It("rollbackConfigurationToSnapshot rollbacks to previous snapshot", func() {
		name := randomString()
		namespace := randomString()

		configMap := getConfigMap(namespace, name)
		secret := getSecret(namespace, name)
		cluster := getCluster(namespace, name)
		clusterProfile := getClusterProfile(name)

		initObjects := []client.Object{configMap, secret, cluster, clusterProfile}
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		snapshotName := randomString()
		snapshotStorage := randomString()
		folder := createDirectoryWithObjects(snapshotName, snapshotStorage,
			[]*unstructured.Unstructured{configMap, secret, cluster, clusterProfile})

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		instance := utils.GetAccessInstance()

		currentConfigMap := &corev1.ConfigMap{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)).To(Succeed())

		originalConfigMapData := make(map[string]string)
		for k := range currentConfigMap.Data {
			originalConfigMapData[k] = currentConfigMap.Data[k]
		}

		currentSecret := &corev1.Secret{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentSecret)).To(Succeed())

		originalSecretData := make(map[string][]byte)
		for k := range currentSecret.Data {
			originalSecretData[k] = currentSecret.Data[k]
		}

		currentCluster := &clusterv1.Cluster{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentCluster)).To(Succeed())

		originalClusterLabels := make(map[string]string)
		for k := range currentCluster.Labels {
			originalClusterLabels[k] = currentCluster.Labels[k]
		}

		currentClusterProfile := &configv1alpha1.ClusterProfile{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Name: name}, currentClusterProfile)).To(Succeed())

		originalClusterProfileSpec := currentClusterProfile.Spec

		updateConfigMapData(currentConfigMap)
		updateSecretData(currentSecret)
		updateClusterLabels(currentCluster)
		updateClusterProfileSpec(currentClusterProfile)

		Expect(snapshot.RollbackConfigurationToSnapshot(context.TODO(), folder, "", "", "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)).To(Succeed())
		Expect(reflect.DeepEqual(currentConfigMap.Data, originalConfigMapData)).To(BeTrue())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentSecret)).To(Succeed())
		Expect(reflect.DeepEqual(currentSecret.Data, originalSecretData)).To(BeTrue())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentCluster)).To(Succeed())
		Expect(reflect.DeepEqual(currentCluster.Labels, originalClusterLabels)).To(BeTrue())

		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Name: name}, currentClusterProfile)).To(Succeed())
		Expect(reflect.DeepEqual(currentClusterProfile.Spec, originalClusterProfileSpec)).To(BeTrue())
	})

	It("getAndRollbackConfigMaps recreates a ConfigMap not existing anymore", func() {
		folder, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())

		name := randomString()
		namespace := randomString()

		collectorClient := collector.GetClient()

		configMap, err := GetUnstructured([]byte(fmt.Sprintf(configMapWithPolicy, namespace, name)))
		Expect(err).To(BeNil())
		Expect(collectorClient.DumpObject(configMap, folder,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		// now read it back it should succeed
		Expect(snapshot.GetAndRollbackConfigMaps(context.TODO(), folder, "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		currentConfigMap := &corev1.ConfigMap{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: namespace, Name: name}, currentConfigMap)).To(Succeed())
	})

	It("getAndRollbackClusterProfiles recreats a ClusterProfile not existing anymore", func() {
		folder, err := os.MkdirTemp("", randomString())
		Expect(err).To(BeNil())

		name := randomString()

		collectorClient := collector.GetClient()

		clusterProfile, err := GetUnstructured([]byte(fmt.Sprintf(clusterProfileTemplate, name)))
		Expect(err).To(BeNil())
		Expect(collectorClient.DumpObject(clusterProfile, folder,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		// now read it back it should succeed
		Expect(snapshot.GetAndRollbackProfiles(context.TODO(), folder, "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		currentClusterProfile := &configv1alpha1.ClusterProfile{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Name: name}, currentClusterProfile)).To(Succeed())
	})
})

func getConfigMap(namespace, name string) *unstructured.Unstructured {
	universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
	configMap := &unstructured.Unstructured{}
	_, _, err := universalDeserializer.Decode([]byte(
		fmt.Sprintf(configMapTemplate, namespace, name)), nil, configMap)
	Expect(err).To(BeNil())
	return configMap
}

func getSecret(namespace, name string) *unstructured.Unstructured {
	universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
	secret := &unstructured.Unstructured{}
	_, _, err := universalDeserializer.Decode([]byte(
		fmt.Sprintf(secretTemplate, namespace, name)), nil, secret)
	Expect(err).To(BeNil())
	return secret
}

func getCluster(namespace, name string) *unstructured.Unstructured {
	universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
	cluster := &unstructured.Unstructured{}
	_, _, err := universalDeserializer.Decode([]byte(
		fmt.Sprintf(clusterTemplate, namespace, name)), nil, cluster)
	Expect(err).To(BeNil())
	return cluster
}

func getClusterProfile(name string) *unstructured.Unstructured {
	universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
	cp := &unstructured.Unstructured{}
	_, _, err := universalDeserializer.Decode([]byte(
		fmt.Sprintf(clusterProfileTemplate, name)), nil, cp)
	Expect(err).To(BeNil())
	return cp
}

func updateConfigMapData(currentConfigMap *corev1.ConfigMap) {
	instance := utils.GetAccessInstance()

	currentConfigMap.Data = map[string]string{
		randomString(): randomString(),
		randomString(): randomString(),
	}
	Expect(instance.UpdateResource(context.TODO(), currentConfigMap)).To(Succeed())
}

func updateSecretData(currentSecret *corev1.Secret) {
	instance := utils.GetAccessInstance()

	currentSecret.Data = map[string][]byte{
		randomString(): []byte(randomString()),
		randomString(): []byte(randomString()),
	}
	Expect(instance.UpdateResource(context.TODO(), currentSecret)).To(Succeed())
}

func updateClusterLabels(currentCluster *clusterv1.Cluster) {
	instance := utils.GetAccessInstance()

	currentCluster.Labels = map[string]string{
		randomString(): randomString(),
		randomString(): randomString(),
		randomString(): randomString(),
	}
	Expect(instance.UpdateResource(context.TODO(), currentCluster)).To(Succeed())
}

func updateClusterProfileSpec(currentClusterProfile *configv1alpha1.ClusterProfile) {
	instance := utils.GetAccessInstance()

	currentClusterProfile.Spec.SyncMode = configv1alpha1.SyncModeDryRun
	currentClusterProfile.Spec.HelmCharts = nil
	Expect(instance.UpdateResource(context.TODO(), currentClusterProfile)).To(Succeed())
}

func createDirectoryWithObjects(snapshotName, snapshotStorage string, objects []*unstructured.Unstructured) string {
	snapshotDir, err := os.MkdirTemp("", randomString())
	timeFormat := "2006-01-02:15:04:05"
	timeFolder := time.Now().Format(timeFormat)
	snapshotDir = filepath.Join(snapshotDir, snapshotStorage, snapshotName, timeFolder)

	By(fmt.Sprintf("Created temporary directory %s", snapshotDir))
	Expect(err).To(BeNil())

	collectorClient := collector.GetClient()

	for i := range objects {
		o := objects[i]
		Expect(addTypeInformationToObject(o)).To(Succeed())
		By(fmt.Sprintf("Adding %s %s/%s to directory %s",
			o.GetObjectKind().GroupVersionKind().GroupKind().Kind, o.GetNamespace(), o.GetName(), snapshotDir))

		Expect(collectorClient.DumpObject(o, snapshotDir,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	}

	return snapshotDir
}

// GetUnstructured returns an unstructured given a []bytes containing it
func GetUnstructured(object []byte) (*unstructured.Unstructured, error) {
	request := &unstructured.Unstructured{}

	universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
	_, _, err := universalDeserializer.Decode(object, nil, request)
	if err != nil {
		return nil, fmt.Errorf("failed to decode k8s resource %s: %w",
			string(object), err)
	}

	return request, nil
}
