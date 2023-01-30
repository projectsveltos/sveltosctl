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

package snapshotter_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	configv1alpha1 "github.com/projectsveltos/sveltos-manager/api/v1alpha1"
	utilsv1alpha1 "github.com/projectsveltos/sveltosctl/api/v1alpha1"
)

func TestSnapshotter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshotter Suite")
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}

func generateSnapshot() *utilsv1alpha1.Snapshot {
	return &utilsv1alpha1.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomString(),
		},
		Spec: utilsv1alpha1.SnapshotSpec{
			Storage: randomString(),
		},
	}
}

func generateChart() *configv1alpha1.Chart {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Chart{
		Namespace:       randomString(),
		ReleaseName:     randomString(),
		RepoURL:         randomString(),
		ChartVersion:    randomString(),
		LastAppliedTime: &t,
	}
}

func generateResource() *configv1alpha1.Resource {
	t := metav1.Time{Time: time.Now()}
	return &configv1alpha1.Resource{
		Namespace:       randomString(),
		Name:            randomString(),
		Group:           randomString(),
		Kind:            randomString(),
		LastAppliedTime: &t,
	}
}

func generateClusterConfiguration() *configv1alpha1.ClusterConfiguration {
	return &configv1alpha1.ClusterConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      randomString(),
			Namespace: randomString(),
		},
		Status: configv1alpha1.ClusterConfigurationStatus{
			ClusterProfileResources: []configv1alpha1.ClusterProfileResource{
				{
					ClusterProfileName: randomString(),
					Features: []configv1alpha1.Feature{
						{
							FeatureID: configv1alpha1.FeatureHelm,
							Charts: []configv1alpha1.Chart{
								*generateChart(), *generateChart(),
							},
						},
						{
							FeatureID: configv1alpha1.FeatureResources,
							Resources: []configv1alpha1.Resource{
								*generateResource(), *generateResource(),
							},
						},
					},
				},
			},
		},
	}
}

func generateClusterProfile() *configv1alpha1.ClusterProfile {
	return &configv1alpha1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomString(),
		},
		Spec: configv1alpha1.ClusterProfileSpec{
			ClusterSelector: libsveltosv1alpha1.Selector("zone:west"),
			SyncMode:        configv1alpha1.SyncModeContinuous,
		},
	}
}
