/*
Copyright 2025. projectsveltos.io. All rights reserved.

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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const testManagementClusterHost = "https://127.0.0.1:6443"

var _ = Describe("Register cluster in pullmode", func() {
	It("prepareApplierYAML returns the YAML to apply to managed cluster", func() {
		clusterNamespace := randomString()
		clusterName := randomString()
		kubeconfig := randomString()

		toApply, err := onboard.PrepareApplierYAML(clusterNamespace, clusterName, kubeconfig,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(strings.Contains(toApply, fmt.Sprintf("--cluster-namespace=%s", clusterNamespace)))
		Expect(strings.Contains(toApply, fmt.Sprintf("--cluster-name=%s", clusterName)))
		Expect(strings.Contains(toApply, "--cluster-namespace=sveltos"))
		Expect(strings.Contains(toApply, fmt.Sprintf("--secret-with-kubeconfig=%s-sveltos-kubeconfig", clusterName)))
	})

	It("onboardSveltosClusterInPullMode updates an existing Role to grant */status permissions", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		// Simulate a Role created by a cluster registered before the */status
		// rules (needed by sveltos-applier to report ClassifierReport/EventReport/
		// HealthCheckReport status back to the management cluster) were added.
		staleRole := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterNamespace,
			},
			Rules: []rbacv1.PolicyRule{
				{
					APIGroups: []string{"lib.projectsveltos.io"},
					Resources: []string{"classifierreports", "eventreports", "healthcheckreports", "reloaderreports"},
					Verbs:     []string{"create", "get", "list", "update", "watch"},
				},
			},
		}

		// registration also re-reads the ServiceAccount token Secret to generate the
		// applier's kubeconfig; a fake client never populates it, so seed it directly.
		staleSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: clusterNamespace,
			},
			Type: corev1.SecretTypeServiceAccountToken,
			Data: map[string][]byte{
				"token":  []byte(randomString()),
				"ca.crt": []byte(randomString()),
			},
		}

		initObjects := []client.Object{staleRole, staleSecret}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, &rest.Config{Host: testManagementClusterHost}, nil, c)

		Expect(onboard.OnboardSveltosClusterInPullMode(context.TODO(), clusterNamespace, clusterName, "", "", "",
			nil, false, textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()
		currentRole := &rbacv1.Role{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, currentRole)).To(Succeed())

		found := false
		for i := range currentRole.Rules {
			rule := currentRole.Rules[i]
			if len(rule.Resources) == 0 || rule.Resources[0] != "classifierreports/status" {
				continue
			}
			found = true
			Expect(rule.Resources).To(ConsistOf("classifierreports/status", "eventreports/status",
				"healthcheckreports/status", "reloaderreports/status"))
			Expect(rule.Verbs).To(ConsistOf("get", "update", "patch"))
		}
		Expect(found).To(BeTrue())
	})

	It("createTokenRenewalRole grants create on serviceaccounts/token scoped to this cluster's ServiceAccount", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		Expect(onboard.CreateTokenRenewalRole(context.TODO(), c, clusterNamespace, clusterName)).To(Succeed())

		role := &rbacv1.Role{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName + onboard.TokenRenewalRBACNamePostfix},
			role)).To(Succeed())

		Expect(role.Rules).To(HaveLen(1))
		Expect(role.Rules[0].APIGroups).To(ConsistOf(""))
		Expect(role.Rules[0].Resources).To(ConsistOf("serviceaccounts/token"))
		Expect(role.Rules[0].ResourceNames).To(ConsistOf(clusterName))
		Expect(role.Rules[0].Verbs).To(ConsistOf("create"))
	})

	It("createTokenRenewalRoleBinding binds sveltoscluster-manager's ServiceAccount, and updates it when sveltosNamespace changes", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		firstNamespace := randomString()
		Expect(onboard.CreateTokenRenewalRoleBinding(context.TODO(), c, clusterNamespace, clusterName, firstNamespace)).
			To(Succeed())

		roleBindingName := clusterName + onboard.TokenRenewalRBACNamePostfix
		roleBinding := &rbacv1.RoleBinding{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: roleBindingName}, roleBinding)).To(Succeed())
		Expect(roleBinding.Subjects).To(HaveLen(1))
		Expect(roleBinding.Subjects[0].Name).To(Equal(onboard.SveltosClusterManagerServiceAccount))
		Expect(roleBinding.Subjects[0].Namespace).To(Equal(firstNamespace))

		// Re-registering with a different --sveltos-namespace must update the binding's subject.
		secondNamespace := randomString()
		Expect(onboard.CreateTokenRenewalRoleBinding(context.TODO(), c, clusterNamespace, clusterName, secondNamespace)).
			To(Succeed())

		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: roleBindingName}, roleBinding)).To(Succeed())
		Expect(roleBinding.Subjects[0].Namespace).To(Equal(secondNamespace))
	})

	It("createSveltosCluster sets TokenRequestRenewalOption only when tokenRenewal is requested", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		Expect(onboard.CreateSveltosCluster(context.TODO(), c, clusterNamespace, clusterName, "", nil, true)).To(Succeed())

		sveltosCluster := &libsveltosv1beta1.SveltosCluster{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sveltosCluster)).To(Succeed())

		Expect(sveltosCluster.Spec.PullMode).To(BeTrue())
		Expect(sveltosCluster.Spec.TokenRequestRenewalOption).ToNot(BeNil())
		Expect(sveltosCluster.Spec.TokenRequestRenewalOption.SAName).To(Equal(clusterName))
		Expect(sveltosCluster.Spec.TokenRequestRenewalOption.SANamespace).To(Equal(clusterNamespace))

		otherClusterName := randomString()
		Expect(onboard.CreateSveltosCluster(context.TODO(), c, clusterNamespace, otherClusterName, "", nil, false)).To(Succeed())

		withoutRenewal := &libsveltosv1beta1.SveltosCluster{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: otherClusterName}, withoutRenewal)).To(Succeed())
		Expect(withoutRenewal.Spec.TokenRequestRenewalOption).To(BeNil())
	})

	It("createManagementClusterURLConfigMap creates and updates the server address and CA data", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		firstURL := "https://" + randomString()
		firstCA := []byte(randomString())
		Expect(onboard.CreateManagementClusterURLConfigMap(context.TODO(), c, clusterNamespace, clusterName,
			firstURL, firstCA)).To(Succeed())

		configMap := &corev1.ConfigMap{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, configMap)).To(Succeed())
		Expect(configMap.Data[onboard.ManagementClusterURLConfigMapKey]).To(Equal(firstURL))
		Expect(configMap.Data[onboard.ManagementClusterCAConfigMapKey]).To(Equal(string(firstCA)))

		// Re-registering with a different --management-cluster-url/CA must update both.
		secondURL := "https://" + randomString()
		secondCA := []byte(randomString())
		Expect(onboard.CreateManagementClusterURLConfigMap(context.TODO(), c, clusterNamespace, clusterName,
			secondURL, secondCA)).To(Succeed())

		Expect(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, configMap)).To(Succeed())
		Expect(configMap.Data[onboard.ManagementClusterURLConfigMapKey]).To(Equal(secondURL))
		Expect(configMap.Data[onboard.ManagementClusterCAConfigMapKey]).To(Equal(string(secondCA)))
	})

	It("deregistration deletes the management-cluster-url ConfigMap", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		Expect(onboard.CreateManagementClusterURLConfigMap(context.TODO(), c, clusterNamespace, clusterName,
			"https://"+randomString(), []byte(randomString()))).To(Succeed())

		Expect(onboard.DeleteConfigMap(context.TODO(), c, clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		Expect(apierrors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, &corev1.ConfigMap{}))).To(BeTrue())
	})

	It("deletePullModeResources also removes the token-renewal Role/RoleBinding", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, &rest.Config{Host: testManagementClusterHost}, nil, c)

		Expect(onboard.CreateTokenRenewalRole(context.TODO(), c, clusterNamespace, clusterName)).To(Succeed())
		Expect(onboard.CreateTokenRenewalRoleBinding(context.TODO(), c, clusterNamespace, clusterName,
			onboard.SveltosClusterManagerNamespaceDefault)).To(Succeed())

		onboard.DeletePullModeResources(context.TODO(), c, clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))

		roleBindingName := clusterName + onboard.TokenRenewalRBACNamePostfix
		Expect(apierrors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: roleBindingName}, &rbacv1.RoleBinding{}))).To(BeTrue())
		Expect(apierrors.IsNotFound(c.Get(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: roleBindingName}, &rbacv1.Role{}))).To(BeTrue())
	})
})
