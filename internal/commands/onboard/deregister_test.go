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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("DeregisterCluster", func() {
	It("deregisterSveltosCluster removes SveltosCluster and kubeconfig Secret in push mode", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		// Create SveltosCluster without PullMode (push mode)
		sveltosCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			Spec: libsveltosv1beta1.SveltosClusterSpec{
				PullMode: false,
			},
		}

		// Create kubeconfig Secret
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				onboard.Kubeconfig: []byte("test-kubeconfig"),
			},
		}

		initObjects := []client.Object{sveltosCluster, secret}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()

		// Verify SveltosCluster is deleted
		tmpCluster := &libsveltosv1beta1.SveltosCluster{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpCluster)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify Secret is deleted
		tmpSecret := &corev1.Secret{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, tmpSecret)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("deregisterSveltosCluster removes all resources in pull mode", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		// Create SveltosCluster with PullMode enabled
		sveltosCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
			Spec: libsveltosv1beta1.SveltosClusterSpec{
				PullMode: true,
			},
		}

		// Create ServiceAccount
		serviceAccount := &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
		}

		// Create ServiceAccount Secret
		saSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
		}

		// Create Role
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
		}

		// Create RoleBinding
		roleBinding := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterName,
			},
		}

		// Create ClusterRole
		clusterRoleName := clusterNamespace + "-" + clusterName
		clusterRole := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleName,
			},
		}

		// Create ClusterRoleBinding
		clusterRoleBinding := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: clusterRoleName,
			},
		}

		// Create kubeconfig Secret
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		kubeconfigSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				onboard.Kubeconfig: []byte("test-kubeconfig"),
			},
		}

		initObjects := []client.Object{
			sveltosCluster,
			serviceAccount,
			saSecret,
			role,
			roleBinding,
			clusterRole,
			clusterRoleBinding,
			kubeconfigSecret,
		}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()

		// Verify SveltosCluster is deleted
		tmpCluster := &libsveltosv1beta1.SveltosCluster{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpCluster)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify ServiceAccount is deleted
		tmpSA := &corev1.ServiceAccount{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpSA)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify Secret is deleted
		tmpSecret := &corev1.Secret{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpSecret)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify Role is deleted
		tmpRole := &rbacv1.Role{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpRole)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify RoleBinding is deleted
		tmpRB := &rbacv1.RoleBinding{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, tmpRB)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify ClusterRole is deleted
		tmpCR := &rbacv1.ClusterRole{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Name: clusterRoleName}, tmpCR)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify ClusterRoleBinding is deleted
		tmpCRB := &rbacv1.ClusterRoleBinding{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Name: clusterRoleName}, tmpCRB)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// Verify kubeconfig Secret is deleted
		tmpKubeconfigSecret := &corev1.Secret{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, tmpKubeconfigSecret)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("deregisterSveltosCluster handles already deleted SveltosCluster gracefully", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		// Should not error when cluster doesn't exist
		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deregisterSveltosCluster cleans up orphaned kubeconfig secret when cluster is not found", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		// Create only the kubeconfig Secret (no SveltosCluster)
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		kubeconfigSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      secretName,
			},
			Data: map[string][]byte{
				onboard.Kubeconfig: []byte("test-kubeconfig"),
			},
		}

		initObjects := []client.Object{kubeconfigSecret}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		// Should not error and should clean up the orphaned secret
		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

		instance := utils.GetAccessInstance()

		// Verify kubeconfig Secret is deleted
		tmpSecret := &corev1.Secret{}
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, tmpSecret)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("deleteServiceAccount handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when ServiceAccount doesn't exist
		Expect(onboard.DeleteServiceAccount(context.TODO(), c, clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deleteSecret handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		secretName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when Secret doesn't exist
		Expect(onboard.DeleteSecret(context.TODO(), c, clusterNamespace, secretName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deleteRole handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		roleName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when Role doesn't exist
		Expect(onboard.DeleteRole(context.TODO(), c, clusterNamespace, roleName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deleteRoleBinding handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		roleBindingName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when RoleBinding doesn't exist
		Expect(onboard.DeleteRoleBinding(context.TODO(), c, clusterNamespace, roleBindingName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deleteClusterRole handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when ClusterRole doesn't exist
		Expect(onboard.DeleteClusterRole(context.TODO(), c, clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})

	It("deleteClusterRoleBinding handles missing resource gracefully", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).Build()

		// Should not error when ClusterRoleBinding doesn't exist
		Expect(onboard.DeleteClusterRoleBinding(context.TODO(), c, clusterNamespace, clusterName,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())
	})
})
