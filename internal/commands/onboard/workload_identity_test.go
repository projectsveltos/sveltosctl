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
	argWorkloadIdentityProvider = "--workload-identity-provider"
	argWorkloadIdentityEndpoint = "--workload-identity-endpoint"
	argWorkloadIdentityCAFile   = "--workload-identity-ca-file"
	argAWSClusterName           = "--aws-cluster-name"
	argAWSRoleARN               = "--aws-role-arn"
	argAWSRegion                = "--aws-region"
	argGCPProjectID             = "--gcp-project-id"
	argGCPClusterName           = "--gcp-cluster-name"
	argGCPLocation              = "--gcp-location"
	argAzureTenantID            = "--azure-tenant-id"
	argAzureClientID            = "--azure-client-id"
	argAzureSubscriptionID      = "--azure-subscription-id"
	argAzureResourceGroup       = "--azure-resource-group"
	argAzureClusterName         = "--azure-cluster-name"
	providerAWS                 = "aws"
	clusterNameAWS              = "my-cluster"
	endpointAWSEKS              = "https://example.eks.amazonaws.com"
	clusterNameEKS              = "my-eks-cluster"
	providerGCP                 = "gcp"
	endpointGCP                 = "https://34.1.2.3"
	projectIDGCP                = "my-project"
	providerAzure               = "azure"
	endpointAzure               = "https://my-aks.hcp.eastus.azmk8s.io"
	tenantIDAzure               = "my-tenant"
)

var _ = Describe("BuildWorkloadIdentityConfig", func() {
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
	_ = logger

	It("returns error when endpoint is missing", func() {
		args := map[string]interface{}{
			argWorkloadIdentityProvider: providerAWS,
			argWorkloadIdentityEndpoint: nil,
			argWorkloadIdentityCAFile:   nil,
			argAWSClusterName:           clusterNameAWS,
			argAWSRoleARN:               nil,
			argAWSRegion:                nil,
			argGCPProjectID:             nil,
			argGCPClusterName:           nil,
			argGCPLocation:              nil,
			argAzureTenantID:            nil,
			argAzureClientID:            nil,
			argAzureSubscriptionID:      nil,
			argAzureResourceGroup:       nil,
			argAzureClusterName:         nil,
		}
		_, _, err := onboard.BuildWorkloadIdentityConfig(args)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(argWorkloadIdentityEndpoint))
	})

	It("returns error for unknown provider", func() {
		args := map[string]interface{}{
			argWorkloadIdentityProvider: "unknown",
			argWorkloadIdentityEndpoint: "https://example.com",
			argWorkloadIdentityCAFile:   nil,
			argAWSClusterName:           nil,
			argAWSRoleARN:               nil,
			argAWSRegion:                nil,
			argGCPProjectID:             nil,
			argGCPClusterName:           nil,
			argGCPLocation:              nil,
			argAzureTenantID:            nil,
			argAzureClientID:            nil,
			argAzureSubscriptionID:      nil,
			argAzureResourceGroup:       nil,
			argAzureClusterName:         nil,
		}
		_, _, err := onboard.BuildWorkloadIdentityConfig(args)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unknown"))
	})

	Context("AWS", func() {
		It("returns error when --aws-cluster-name is missing", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerAWS,
				argWorkloadIdentityEndpoint: endpointAWSEKS,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           nil,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             nil,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            nil,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			_, _, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(argAWSClusterName))
		})

		It("builds AWS config with required fields only", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerAWS,
				argWorkloadIdentityEndpoint: endpointAWSEKS,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           clusterNameEKS,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             nil,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            nil,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			wi, caFile, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(BeNil())
			Expect(caFile).To(BeEmpty())
			Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderAWS))
			Expect(wi.Endpoint).To(Equal(endpointAWSEKS))
			Expect(wi.AWS).ToNot(BeNil())
			Expect(wi.AWS.ClusterName).To(Equal(clusterNameEKS))
			Expect(wi.AWS.RoleARN).To(BeEmpty())
			Expect(wi.AWS.Region).To(BeEmpty())
		})

		It("builds AWS config with all optional fields", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerAWS,
				argWorkloadIdentityEndpoint: endpointAWSEKS,
				argWorkloadIdentityCAFile:   "/tmp/ca.crt",
				argAWSClusterName:           clusterNameEKS,
				argAWSRoleARN:               "arn:aws:iam::123456789012:role/my-role",
				argAWSRegion:                "us-east-1",
				argGCPProjectID:             nil,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            nil,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			wi, caFile, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(BeNil())
			Expect(caFile).To(Equal("/tmp/ca.crt"))
			Expect(wi.AWS.RoleARN).To(Equal("arn:aws:iam::123456789012:role/my-role"))
			Expect(wi.AWS.Region).To(Equal("us-east-1"))
		})
	})

	Context("GCP", func() {
		It("returns error when required GCP fields are missing", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerGCP,
				argWorkloadIdentityEndpoint: endpointGCP,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           nil,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             projectIDGCP,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            nil,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			_, _, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(argGCPClusterName))
		})

		It("builds GCP config with all required fields", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerGCP,
				argWorkloadIdentityEndpoint: endpointGCP,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           nil,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             projectIDGCP,
				argGCPClusterName:           "cluster-managed",
				argGCPLocation:              "us-central1-a",
				argAzureTenantID:            nil,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			wi, caFile, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(BeNil())
			Expect(caFile).To(BeEmpty())
			Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderGCP))
			Expect(wi.GCP).ToNot(BeNil())
			Expect(wi.GCP.ProjectID).To(Equal(projectIDGCP))
			Expect(wi.GCP.ClusterName).To(Equal("cluster-managed"))
			Expect(wi.GCP.Location).To(Equal("us-central1-a"))
		})
	})

	Context("Azure", func() {
		It("returns error when required Azure fields are missing", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerAzure,
				argWorkloadIdentityEndpoint: endpointAzure,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           nil,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             nil,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            tenantIDAzure,
				argAzureClientID:            nil,
				argAzureSubscriptionID:      nil,
				argAzureResourceGroup:       nil,
				argAzureClusterName:         nil,
			}
			_, _, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(argAzureClientID))
		})

		It("builds Azure config with required fields and optional fields", func() {
			args := map[string]interface{}{
				argWorkloadIdentityProvider: providerAzure,
				argWorkloadIdentityEndpoint: endpointAzure,
				argWorkloadIdentityCAFile:   nil,
				argAWSClusterName:           nil,
				argAWSRoleARN:               nil,
				argAWSRegion:                nil,
				argGCPProjectID:             nil,
				argGCPClusterName:           nil,
				argGCPLocation:              nil,
				argAzureTenantID:            tenantIDAzure,
				argAzureClientID:            "my-client",
				argAzureSubscriptionID:      "my-sub",
				argAzureResourceGroup:       "my-rg",
				argAzureClusterName:         "my-aks",
			}
			wi, caFile, err := onboard.BuildWorkloadIdentityConfig(args)
			Expect(err).To(BeNil())
			Expect(caFile).To(BeEmpty())
			Expect(wi.Provider).To(Equal(libsveltosv1beta1.WorkloadIdentityProviderAzure))
			Expect(wi.Azure).ToNot(BeNil())
			Expect(wi.Azure.TenantID).To(Equal(tenantIDAzure))
			Expect(wi.Azure.ClientID).To(Equal("my-client"))
			Expect(wi.Azure.SubscriptionID).To(Equal("my-sub"))
			Expect(wi.Azure.ResourceGroup).To(Equal("my-rg"))
			Expect(wi.Azure.ClusterName).To(Equal("my-aks"))
		})
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
			Endpoint: endpointAWSEKS,
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

		// no kubeconfig secret created
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
		// first register with kubeconfig
		Expect(onboard.OnboardSveltosCluster(
			context.TODO(), clusterNamespace, clusterName, "", []byte("kubeconfig-data"), nil, false, logger,
		)).To(Succeed())

		// then re-register with workload identity
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
			Endpoint:    endpointAWSEKS,
			CASecretRef: &corev1.LocalObjectReference{Name: caSecretName},
			AWS:         &libsveltosv1beta1.AWSWorkloadIdentityConfig{ClusterName: clusterNameAWS},
		}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sc, caSecret).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		Expect(onboard.DeregisterSveltosCluster(context.TODO(), clusterNamespace, clusterName, logger)).To(Succeed())

		instance := utils.GetAccessInstance()

		// SveltosCluster deleted
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: clusterName}, &libsveltosv1beta1.SveltosCluster{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// CA secret deleted
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: caSecretName}, &corev1.Secret{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())

		// kubeconfig secret was never created — still not found (not an error condition)
		kubeconfigSecretName := clusterName + onboard.SveltosKubeconfigSecretNamePostfix
		err = instance.GetResource(context.TODO(),
			types.NamespacedName{Namespace: clusterNamespace, Name: kubeconfigSecretName}, &corev1.Secret{})
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})
})
