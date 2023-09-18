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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/klogr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Register Mgmt Cluster", func() {
	It("createSveltosCluster creates SveltosCluster", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())

		clusterNamespace := randomString()
		clusterName := randomString()

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterNamespace,
			},
		}

		initObjects := []client.Object{ns}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.CreateSveltosCluster(context.TODO(),
			clusterNamespace, clusterName, klogr.New())).To(Succeed())

		currentSveltosCluster := &libsveltosv1alpha1.SveltosCluster{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName},
			currentSveltosCluster)).To(Succeed())

		Expect(onboard.CreateSveltosCluster(context.TODO(),
			clusterNamespace, clusterName, klogr.New())).To(Succeed())
	})

	It("createNamespace creates namespace", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		ns := randomString()
		Expect(onboard.CreateNamespace(context.TODO(), ns, klogr.New())).To(Succeed())

		currentNs := &corev1.Namespace{}
		err = c.Get(context.TODO(), types.NamespacedName{Name: ns}, currentNs)
		// Only defaultNamespace is created. Any other namespace is expected to be created
		// by admin as per any other cluster registration
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		Expect(onboard.CreateNamespace(context.TODO(), onboard.DefaultNamespace, klogr.New())).To(Succeed())
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Name: onboard.DefaultNamespace}, currentNs)).To(Succeed())

		Expect(onboard.CreateNamespace(context.TODO(), onboard.DefaultNamespace, klogr.New())).To(Succeed())
	})

	It("createClusterRole creates ClusterRole", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.CreateClusterRole(context.TODO(), klogr.New())).To(Succeed())

		currentClusterRole := &rbacv1.ClusterRole{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: onboard.ClusterRoleName},
			currentClusterRole)).To(Succeed())

		Expect(onboard.CreateClusterRole(context.TODO(), klogr.New())).To(Succeed())
	})

	It("createClusterRoleBinding creates ClusterRoleBinding", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.CreateClusterRoleBinding(context.TODO(), klogr.New())).To(Succeed())

		currentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: onboard.ClusterRoleBindingName},
			currentClusterRoleBinding)).To(Succeed())

		Expect(currentClusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
		Expect(currentClusterRoleBinding.RoleRef.Name).To(Equal(onboard.ClusterRoleName))

		Expect(len(currentClusterRoleBinding.Subjects)).To(Equal(1))
		Expect(currentClusterRoleBinding.Subjects[0].Name).To(Equal(onboard.SaName))
		Expect(currentClusterRoleBinding.Subjects[0].Namespace).To(Equal(onboard.SaNamespace))
		Expect(currentClusterRoleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))

		Expect(onboard.CreateClusterRoleBinding(context.TODO(), klogr.New())).To(Succeed())
	})
})
