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

package utils_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("ClusterConfiguration", func() {
	//nolint: dupl // exception for a test
	It("ListClusterConfigurations returns list of all clusterConfigurations", func() {
		initObjects := []client.Object{}

		for i := 0; i < 10; i++ {
			clusterConfiguration := &configv1beta1.ClusterConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      randomString(),
					Namespace: randomString(),
				},
			}
			initObjects = append(initObjects, clusterConfiguration)
		}

		clusterConfiguration := &configv1beta1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}
		initObjects = append(initObjects, clusterConfiguration)

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		clusterConfigurations, err := k8sAccess.ListClusterConfigurations(context.TODO(), "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(clusterConfigurations.Items)).To(Equal(len(initObjects)))

		clusterConfigurations, err = k8sAccess.ListClusterConfigurations(context.TODO(), clusterConfiguration.Namespace,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(len(clusterConfigurations.Items)).To(Equal(1))
	})

	It("GetClusterConfiguration returns ClusterConfiguration for a given cluster ", func() {
		clusterConfiguration := &configv1beta1.ClusterConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
		}

		initObjects := []client.Object{clusterConfiguration}
		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		k8sAccess := utils.GetK8sAccess(scheme, c)

		_, err := k8sAccess.GetClusterConfiguration(context.TODO(), randomString(), randomString(),
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		cc, err := k8sAccess.GetClusterConfiguration(context.TODO(),
			clusterConfiguration.Namespace, clusterConfiguration.Name,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(cc).ToNot(BeNil())
		Expect(cc.Namespace).To(Equal(clusterConfiguration.Namespace))
		Expect(cc.Name).To(Equal(clusterConfiguration.Name))
	})

	It("GetHelmReleases returns deployed helm releases", func() {
		chart1 := &configv1beta1.Chart{
			RepoURL:         randomString(),
			ReleaseName:     randomString(),
			Namespace:       randomString(),
			ChartVersion:    randomString(),
			LastAppliedTime: &metav1.Time{Time: time.Now()},
		}

		chart2 := &configv1beta1.Chart{
			RepoURL:         randomString(),
			ReleaseName:     randomString(),
			Namespace:       randomString(),
			ChartVersion:    randomString(),
			LastAppliedTime: &metav1.Time{Time: time.Now()},
		}

		clusterConfiguration := createClusterConfiguration(nil, nil,
			[]configv1beta1.Chart{*chart1}, []configv1beta1.Chart{*chart1, *chart2})
		initObjects := []client.Object{clusterConfiguration}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		chartMap := k8sAccess.GetHelmReleases(clusterConfiguration,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(chartMap).ToNot(BeNil())
		Expect(len(chartMap)).To(Equal(2))
		Expect(len(chartMap[*chart1])).To(Equal(2))
		Expect(len(chartMap[*chart2])).To(Equal(1))
	})

	It("GetResources returns deployed resources", func() {
		resource1 := &libsveltosv1beta1.Resource{
			Name:            randomString(),
			Namespace:       randomString(),
			Group:           randomString(),
			Kind:            randomString(),
			LastAppliedTime: &metav1.Time{Time: time.Now()},
		}

		resource2 := &libsveltosv1beta1.Resource{
			Name:            randomString(),
			Namespace:       randomString(),
			Group:           randomString(),
			Kind:            randomString(),
			LastAppliedTime: &metav1.Time{Time: time.Now()},
		}

		clusterConfiguration := createClusterConfiguration(
			[]libsveltosv1beta1.Resource{*resource1},
			[]libsveltosv1beta1.Resource{*resource1, *resource2},
			nil, nil)
		initObjects := []client.Object{clusterConfiguration}

		scheme := runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		k8sAccess := utils.GetK8sAccess(scheme, c)
		resourceMap := k8sAccess.GetResources(clusterConfiguration,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(resourceMap).ToNot(BeNil())
		Expect(len(resourceMap)).To(Equal(2))
		Expect(len(resourceMap[*resource1])).To(Equal(2))
		Expect(len(resourceMap[*resource2])).To(Equal(1))
	})
})

func createClusterConfiguration(clusterProfile1Resources, clusterProfile2Resources []libsveltosv1beta1.Resource,
	clusterProfile1Charts, clusterProfile2Charts []configv1beta1.Chart) *configv1beta1.ClusterConfiguration {

	cfr1 := &configv1beta1.ClusterProfileResource{
		ClusterProfileName: randomString(),
		Features: []configv1beta1.Feature{
			{
				FeatureID: libsveltosv1beta1.FeatureHelm,
				Resources: clusterProfile1Resources,
				Charts:    clusterProfile1Charts,
			},
		},
	}

	cfr2 := &configv1beta1.ClusterProfileResource{
		ClusterProfileName: randomString(),
		Features: []configv1beta1.Feature{
			{
				FeatureID: libsveltosv1beta1.FeatureHelm,
				Resources: clusterProfile2Resources,
				Charts:    clusterProfile2Charts,
			},
		},
	}

	return &configv1beta1.ClusterConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      randomString(),
			Namespace: randomString(),
		},
		Status: configv1beta1.ClusterConfigurationStatus{
			ClusterProfileResources: []configv1beta1.ClusterProfileResource{
				*cfr1,
				*cfr2,
			},
		},
	}
}
