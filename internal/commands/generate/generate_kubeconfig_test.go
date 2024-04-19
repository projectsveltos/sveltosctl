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

package generate_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/projectsveltos/sveltosctl/internal/commands/generate"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Register Mgmt Cluster", func() {
	It("createNamespace creates namespace", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		ns := randomString()
		Expect(generate.CreateNamespace(context.TODO(), c, ns,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		currentNs := &corev1.Namespace{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: ns}, currentNs)).To(BeNil())
	})

	It("createClusterRole creates ClusterRole", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(generate.CreateClusterRole(context.TODO(), c, generate.Projectsveltos,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		currentClusterRole := &rbacv1.ClusterRole{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: generate.Projectsveltos},
			currentClusterRole)).To(Succeed())

		Expect(generate.CreateClusterRole(context.TODO(), c, generate.Projectsveltos,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("createClusterRoleBinding creates ClusterRoleBinding", func() {
		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		saNamespace := randomString()
		saName := randomString()
		Expect(generate.CreateClusterRoleBinding(context.TODO(), c, generate.Projectsveltos,
			generate.Projectsveltos, saNamespace, saName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		currentClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: generate.Projectsveltos},
			currentClusterRoleBinding)).To(Succeed())

		Expect(currentClusterRoleBinding.RoleRef.Kind).To(Equal("ClusterRole"))
		Expect(currentClusterRoleBinding.RoleRef.Name).To(Equal(generate.Projectsveltos))

		Expect(len(currentClusterRoleBinding.Subjects)).To(Equal(1))
		Expect(currentClusterRoleBinding.Subjects[0].Name).To(Equal(saName))
		Expect(currentClusterRoleBinding.Subjects[0].Namespace).To(Equal(saNamespace))
		Expect(currentClusterRoleBinding.Subjects[0].Kind).To(Equal("ServiceAccount"))

		Expect(generate.CreateClusterRoleBinding(context.TODO(), c, generate.Projectsveltos,
			generate.Projectsveltos, saNamespace, saName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})
})
