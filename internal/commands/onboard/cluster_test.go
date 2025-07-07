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

package onboard_test

import (
	"context"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("OnboardCluster", func() {
	It("onboardSveltosCluster creates SveltosCluster and Secret", func() {
		data := randomString()

		kubeconfigData := []byte(data)

		clusterNamespace := randomString()
		clusterName := randomString()

		initObjects := []client.Object{}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		labels := map[string]string{
			randomString(): randomString(),
			randomString(): randomString(),
		}

		Expect(onboard.OnboardSveltosCluster(context.TODO(), clusterNamespace, clusterName, kubeconfigData,
			labels, false, textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()

		sveltosCluster := &libsveltosv1beta1.SveltosCluster{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)
		Expect(err).To(BeNil())
		Expect(reflect.DeepEqual(sveltosCluster.Labels, labels)).To(BeTrue())

		secret := &corev1.Secret{}
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		err = instance.GetResource(context.TODO(), types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
		Expect(err).To(BeNil())

		Expect(secret.Data).ToNot(BeNil())
		Expect(secret.Data[onboard.Kubeconfig]).To(Equal([]byte(data)))

		// verify operation updates existing resources
		labels = map[string]string{
			randomString(): randomString(),
			randomString(): randomString(),
		}

		Expect(onboard.OnboardSveltosCluster(context.TODO(), clusterNamespace, clusterName, kubeconfigData,
			labels, false, textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})
})
