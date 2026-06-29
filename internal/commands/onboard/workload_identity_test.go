/*
Copyright 2026. projectsveltos.io. All rights reserved.

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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	endpointEKS    = "https://example.eks.amazonaws.com"
	clusterNameEKS = "my-eks-cluster"
	endpointGKE    = "https://34.1.2.3"
	projectIDGKE   = "my-project"
	endpointAKS    = "https://my-aks.hcp.eastus.azmk8s.io"
	tenantIDAKS    = "my-tenant"
	clientIDAKS    = "my-client"
)

var _ = Describe("BuildEKSWorkloadIdentityConfig", func() {
	It("returns error when endpoint is missing", func() {
		_, err := onboard.BuildEKSWorkloadIdentityConfig("", clusterNameEKS, "", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--endpoint"))
	})

	It("returns error when eks-cluster-name is missing", func() {
		_, err := onboard.BuildEKSWorkloadIdentityConfig(endpointEKS, "", "", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--eks-cluster-name"))
	})

	It("builds config with required fields only", func() {
		wi, err := onboard.BuildEKSWorkloadIdentityConfig(endpointEKS, clusterNameEKS, "", "")
		Expect(err).To(BeNil())
		Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderAWS))
		Expect(wi.Endpoint).To(Equal(endpointEKS))
		Expect(wi.AWS).ToNot(BeNil())
		Expect(wi.AWS.ClusterName).To(Equal(clusterNameEKS))
		Expect(wi.AWS.RoleARN).To(BeEmpty())
		Expect(wi.AWS.Region).To(BeEmpty())
	})

	It("builds config with all optional fields", func() {
		wi, err := onboard.BuildEKSWorkloadIdentityConfig(endpointEKS, clusterNameEKS,
			"arn:aws:iam::123456789012:role/my-role", "us-east-1")
		Expect(err).To(BeNil())
		Expect(wi.AWS.RoleARN).To(Equal("arn:aws:iam::123456789012:role/my-role"))
		Expect(wi.AWS.Region).To(Equal("us-east-1"))
	})
})

var _ = Describe("BuildGKEWorkloadIdentityConfig", func() {
	It("returns error when endpoint is missing", func() {
		_, err := onboard.BuildGKEWorkloadIdentityConfig("", projectIDGKE, "my-cluster", "us-central1-a")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--endpoint"))
	})

	It("returns error when project-id is missing", func() {
		_, err := onboard.BuildGKEWorkloadIdentityConfig(endpointGKE, "", "my-cluster", "us-central1-a")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--project-id"))
	})

	It("returns error when gke-cluster-name is missing", func() {
		_, err := onboard.BuildGKEWorkloadIdentityConfig(endpointGKE, projectIDGKE, "", "us-central1-a")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--gke-cluster-name"))
	})

	It("returns error when location is missing", func() {
		_, err := onboard.BuildGKEWorkloadIdentityConfig(endpointGKE, projectIDGKE, "my-cluster", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--location"))
	})

	It("builds config with all required fields", func() {
		wi, err := onboard.BuildGKEWorkloadIdentityConfig(endpointGKE, projectIDGKE, "cluster-managed", "us-central1-a")
		Expect(err).To(BeNil())
		Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderGCP))
		Expect(wi.Endpoint).To(Equal(endpointGKE))
		Expect(wi.GCP).ToNot(BeNil())
		Expect(wi.GCP.ProjectID).To(Equal(projectIDGKE))
		Expect(wi.GCP.ClusterName).To(Equal("cluster-managed"))
		Expect(wi.GCP.Location).To(Equal("us-central1-a"))
	})
})

var _ = Describe("BuildAKSWorkloadIdentityConfig", func() {
	It("returns error when endpoint is missing", func() {
		_, err := onboard.BuildAKSWorkloadIdentityConfig("", tenantIDAKS, clientIDAKS, "", "", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--endpoint"))
	})

	It("returns error when tenant-id is missing", func() {
		_, err := onboard.BuildAKSWorkloadIdentityConfig(endpointAKS, "", clientIDAKS, "", "", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--tenant-id"))
	})

	It("returns error when client-id is missing", func() {
		_, err := onboard.BuildAKSWorkloadIdentityConfig(endpointAKS, tenantIDAKS, "", "", "", "")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("--client-id"))
	})

	It("builds config with required fields only", func() {
		wi, err := onboard.BuildAKSWorkloadIdentityConfig(endpointAKS, tenantIDAKS, clientIDAKS, "", "", "")
		Expect(err).To(BeNil())
		Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderAzure))
		Expect(wi.Endpoint).To(Equal(endpointAKS))
		Expect(wi.Azure).ToNot(BeNil())
		Expect(wi.Azure.TenantID).To(Equal(tenantIDAKS))
		Expect(wi.Azure.ClientID).To(Equal(clientIDAKS))
		Expect(wi.Azure.SubscriptionID).To(BeEmpty())
		Expect(wi.Azure.ResourceGroup).To(BeEmpty())
		Expect(wi.Azure.ClusterName).To(BeEmpty())
	})

	It("builds config with all optional fields", func() {
		wi, err := onboard.BuildAKSWorkloadIdentityConfig(endpointAKS, tenantIDAKS, clientIDAKS,
			"my-sub", "my-rg", "my-aks")
		Expect(err).To(BeNil())
		Expect(wi.Azure.SubscriptionID).To(Equal("my-sub"))
		Expect(wi.Azure.ResourceGroup).To(Equal("my-rg"))
		Expect(wi.Azure.ClusterName).To(Equal("my-aks"))
	})
})

var _ = Describe("OnboardSveltosClusterWithWorkloadIdentity", func() {
	var (
		clusterNamespace string
		clusterName      string
		logger           = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
	)

	BeforeEach(func() {
		clusterNamespace = randomString()
		clusterName = randomString()

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects([]client.Object{}...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
	})

	awsWI := func() *libsveltosv1beta1.WorkloadIdentityConfig {
		return &libsveltosv1beta1.WorkloadIdentityConfig{
			Provider: libsveltosv1beta1.WorkloadIdentityProviderAWS,
			Endpoint: endpointEKS,
			AWS: &libsveltosv1beta1.AWSWorkloadIdentityConfig{
				ClusterName: clusterNameEKS,
			},
		}
	}

	It("creates SveltosCluster with workloadIdentity and no kubeconfig Secret", func() {
		Expect(onboard.OnboardSveltosClusterWithWorkloadIdentity(
			context.TODO(), clusterNamespace, clusterName, "", awsWI(), "", nil, logger,
		)).To(Succeed())

		instance := utils.GetAccessInstance()

		sc := &libsveltosv1beta1.SveltosCluster{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sc)).To(Succeed())
		Expect(sc.Spec.WorkloadIdentity).ToNot(BeNil())
		Expect(sc.Spec.WorkloadIdentity.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderAWS))
		Expect(sc.Spec.KubeconfigKeyName).To(BeEmpty())

		secret := &corev1.Secret{}
		secretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		err := instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: secretName}, secret)
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("creates CA Secret and sets CASecretRef when ca-file is provided", func() {
		caData := []byte("fake-ca-cert")
		f, err := os.CreateTemp("", "ca-*.crt")
		Expect(err).To(BeNil())
		defer os.Remove(f.Name())
		_, err = f.Write(caData)
		Expect(err).To(BeNil())
		Expect(f.Close()).To(Succeed())

		Expect(onboard.OnboardSveltosClusterWithWorkloadIdentity(
			context.TODO(), clusterNamespace, clusterName, "", awsWI(), f.Name(), nil, logger,
		)).To(Succeed())

		instance := utils.GetAccessInstance()

		sc := &libsveltosv1beta1.SveltosCluster{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sc)).To(Succeed())
		Expect(sc.Spec.WorkloadIdentity.CASecretRef).ToNot(BeNil())

		caSecretName := clusterName + onboard.SveltosCASecretNamePostfix
		Expect(sc.Spec.WorkloadIdentity.CASecretRef.Name).To(Equal(caSecretName))

		caSecret := &corev1.Secret{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: caSecretName}, caSecret)).To(Succeed())
		Expect(caSecret.Data[onboard.CAKey]).To(Equal(caData))
	})

	It("updates existing SveltosCluster and clears KubeconfigKeyName", func() {
		Expect(onboard.OnboardSveltosCluster(
			context.TODO(), clusterNamespace, clusterName, "", []byte("kubeconfig-data"), nil, false, logger,
		)).To(Succeed())

		Expect(onboard.OnboardSveltosClusterWithWorkloadIdentity(
			context.TODO(), clusterNamespace, clusterName, "", awsWI(), "", nil, logger,
		)).To(Succeed())

		instance := utils.GetAccessInstance()
		sc := &libsveltosv1beta1.SveltosCluster{}
		Expect(instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, sc)).To(Succeed())
		Expect(sc.Spec.WorkloadIdentity).ToNot(BeNil())
		Expect(sc.Spec.KubeconfigKeyName).To(BeEmpty())
	})
})

var _ = Describe("DeregisterSveltosCluster workload identity", func() {
	var (
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
	)

	It("deletes CA Secret but not kubeconfig Secret for workload identity cluster", func() {
		clusterNamespace := randomString()
		clusterName := randomString()

		caSecretName := clusterName + onboard.SveltosCASecretNamePostfix
		caSecret := &corev1.Secret{}
		caSecret.Namespace = clusterNamespace
		caSecret.Name = caSecretName
		caSecret.Data = map[string][]byte{onboard.CAKey: []byte("ca-data")}

		sc := &libsveltosv1beta1.SveltosCluster{}
		sc.Namespace = clusterNamespace
		sc.Name = clusterName
		sc.Spec.WorkloadIdentity = &libsveltosv1beta1.WorkloadIdentityConfig{
			Provider:    libsveltosv1beta1.WorkloadIdentityProviderAWS,
			Endpoint:    endpointEKS,
			CASecretRef: &corev1.LocalObjectReference{Name: caSecretName},
			AWS:         &libsveltosv1beta1.AWSWorkloadIdentityConfig{ClusterName: clusterNameEKS},
		}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc, caSecret).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName, logger)).To(Succeed())

		instance := utils.GetAccessInstance()

		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, &libsveltosv1beta1.SveltosCluster{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: caSecretName}, &corev1.Secret{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		kubeconfigSecretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: kubeconfigSecretName}, &corev1.Secret{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
