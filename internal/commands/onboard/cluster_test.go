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
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/utils"

	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
)

var _ = Describe("OnboardCluster", func() {
	It("onboardSveltosCluster creates SveltosCluster and Secret", func() {
		data := randomString()

		// Create temp file
		kubeconfigFile, err := os.CreateTemp("", "kubeconfig")
		Expect(err).To(BeNil())

		defer os.Remove(kubeconfigFile.Name())

		_, err = kubeconfigFile.WriteString(data)
		Expect(err).To(BeNil())

		clusterNamespace := randomString()
		clusterName := randomString()

		initObjects := []client.Object{}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.OnboardSveltosCluster(context.TODO(), clusterNamespace, clusterName,
			kubeconfigFile.Name(), textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()

		sveltosCluster := &libsveltosv1alpha1.SveltosCluster{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)
		Expect(err).To(BeNil())

		secret := &corev1.Secret{}
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		err = instance.GetResource(context.TODO(), types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
		Expect(err).To(BeNil())

		Expect(secret.Data).ToNot(BeNil())
		Expect(secret.Data["value"]).To(Equal([]byte(data)))
	})
})
