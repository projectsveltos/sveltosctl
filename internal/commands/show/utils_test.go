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

package show_test

import (
	"fmt"
	"time"
	"unicode/utf8"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/controller-runtime/pkg/client"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

// addDeployedHelmCharts adds provided charts as deployed in clusterConfiguration status
func addDeployedHelmCharts(clusterConfiguration *configv1alpha1.ClusterConfiguration,
	clusterProfileName string, charts []configv1alpha1.Chart) *configv1alpha1.ClusterConfiguration {

	if clusterConfiguration.Status.ClusterProfileResources == nil {
		clusterConfiguration.Status.ClusterProfileResources = make([]configv1alpha1.ClusterProfileResource, 0)
	}

	for i := range clusterConfiguration.Status.ClusterProfileResources {
		cfr := &clusterConfiguration.Status.ClusterProfileResources[i]
		if cfr.ClusterProfileName == clusterProfileName {
			if cfr.Features == nil {
				cfr.Features = make([]configv1alpha1.Feature, 0)
			}
			cfr.Features = append(cfr.Features,
				configv1alpha1.Feature{
					FeatureID: configv1alpha1.FeatureHelm,
					Charts:    charts,
				})

			return clusterConfiguration
		}
	}

	cfr := &configv1alpha1.ClusterProfileResource{
		ClusterProfileName: clusterProfileName,
		Features: []configv1alpha1.Feature{
			{FeatureID: configv1alpha1.FeatureHelm, Charts: charts},
		},
	}
	clusterConfiguration.Status.ClusterProfileResources = append(clusterConfiguration.Status.ClusterProfileResources, *cfr)

	return clusterConfiguration
}

// addDeployedResources adds provided resources as deployed in clusterConfiguration status
func addDeployedResources(clusterConfiguration *configv1alpha1.ClusterConfiguration,
	clusterProfileName string, resources []configv1alpha1.Resource) *configv1alpha1.ClusterConfiguration {

	if clusterConfiguration.Status.ClusterProfileResources == nil {
		clusterConfiguration.Status.ClusterProfileResources = make([]configv1alpha1.ClusterProfileResource, 0)
	}

	for i := range clusterConfiguration.Status.ClusterProfileResources {
		cfr := &clusterConfiguration.Status.ClusterProfileResources[i]
		if cfr.ClusterProfileName == clusterProfileName {
			if cfr.Features == nil {
				cfr.Features = make([]configv1alpha1.Feature, 0)
			}
			cfr.Features = append(cfr.Features,
				configv1alpha1.Feature{
					FeatureID: configv1alpha1.FeatureResources,
					Resources: resources,
				})

			return clusterConfiguration
		}
	}

	cfr := &configv1alpha1.ClusterProfileResource{
		ClusterProfileName: clusterProfileName,
		Features: []configv1alpha1.Feature{
			{FeatureID: configv1alpha1.FeatureResources, Resources: resources},
		},
	}
	clusterConfiguration.Status.ClusterProfileResources = append(clusterConfiguration.Status.ClusterProfileResources, *cfr)

	return clusterConfiguration
}

func generateChart() *configv1alpha1.Chart {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Chart{
		RepoURL:         randomString(),
		ReleaseName:     randomString(),
		Namespace:       randomString(),
		ChartVersion:    randomString(),
		LastAppliedTime: &t,
	}
}

func generateResource() *configv1alpha1.Resource {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Resource{
		Name:            randomString(),
		Namespace:       randomString(),
		Group:           randomString(),
		Kind:            randomString(),
		LastAppliedTime: &t,
	}
}

func generateReleaseReport(action string) *configv1alpha1.ReleaseReport {
	return &configv1alpha1.ReleaseReport{
		ReleaseName:      randomString(),
		ReleaseNamespace: randomString(),
		ChartVersion:     randomString(),
		Action:           action,
	}
}

func generateResourceReport(action string) *configv1alpha1.ResourceReport {
	return &configv1alpha1.ResourceReport{
		Resource: configv1alpha1.Resource{
			Name:      randomString(),
			Namespace: randomString(),
			Group:     randomString(),
			Kind:      randomString(),
		},
		Action: action,
	}
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}

// createConfigMapWithPolicy creates a configMap with Data policies
func createConfigMapWithPolicy(namespace, configMapName string, policyStrs ...string) *corev1.ConfigMap {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      configMapName,
		},
		Data: map[string]string{},
	}
	for i := range policyStrs {
		key := fmt.Sprintf("policy%d.yaml", i)
		if utf8.Valid([]byte(policyStrs[i])) {
			cm.Data[key] = policyStrs[i]
		} else {
			cm.BinaryData[key] = []byte(policyStrs[i])
		}
	}

	Expect(addTypeInformationToObject(cm)).To(Succeed())

	return cm
}

// createSecretWithPolicy creates a Secret with Data containing base64 encoded policies
func createSecretWithPolicy(namespace, configMapName string, policyStrs ...string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      configMapName,
		},
		Data: map[string][]byte{},
	}
	for i := range policyStrs {
		key := fmt.Sprintf("policy%d.yaml", i)
		secret.Data[key] = []byte(policyStrs[i])
	}

	Expect(addTypeInformationToObject(secret)).To(Succeed())

	return secret
}

func addTypeInformationToObject(obj client.Object) error {
	scheme, err := utils.GetScheme()
	if err != nil {
		return err
	}

	gvks, _, err := scheme.ObjectKinds(obj)
	if err != nil {
		return err
	}

	for _, gvk := range gvks {
		if gvk.Kind == "" {
			continue
		}
		if gvk.Version == "" || gvk.Version == runtime.APIVersionInternal {
			continue
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		break
	}

	return nil
}
