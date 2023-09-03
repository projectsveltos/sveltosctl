/*
Copyright 2023. projectsveltos.io. All rights reserved.

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

package collector_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/cluster-api/util"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

func TestSnapshotter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Snapshotter Suite")
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}

func generateClusterProfile() *configv1beta1.ClusterProfile {
	return &configv1beta1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomString(),
		},
		Spec: configv1beta1.Spec{
			ClusterSelector: libsveltosv1beta1.Selector{
				LabelSelector: metav1.LabelSelector{
					MatchLabels: map[string]string{"zone": "west"},
				},
			},
			SyncMode: configv1beta1.SyncModeContinuous,
		},
	}
}

func generateClusterConfiguration() *configv1beta1.ClusterConfiguration {
	return &configv1beta1.ClusterConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      randomString(),
			Namespace: randomString(),
		},
		Status: configv1beta1.ClusterConfigurationStatus{
			ClusterProfileResources: []configv1beta1.ClusterProfileResource{
				{
					ClusterProfileName: randomString(),
				},
			},
		},
	}
}
