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
	"fmt"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/klogr"
	kubectlscheme "k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	nsInstanceTemplate = `apiVersion: v1
kind: Namespace
metadata:
  name: %s
`

	nsInstanceTemplateWithLabels = `apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    zone: us
`
)

var _ = Describe("CRUD", func() {
	var c client.Client
	var scheme *runtime.Scheme
	var testEnv *envtest.Environment
	var cfg *rest.Config

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(utils.AddToScheme(scheme)).To(Succeed())
		c = fake.NewClientBuilder().WithScheme(scheme).Build()

		testEnv = &envtest.Environment{
			CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")},
		}

		var err error
		cfg, err = testEnv.Start()
		Expect(err).To(BeNil())
	})

	AfterEach(func() {
		Expect(testEnv.Stop()).To(Succeed())
	})

	It("CreateResource creates a resource", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
		}

		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.CreateResource(context.TODO(), ns)).To(Succeed())
		currentNamespace := &corev1.Namespace{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: ns.Name}, currentNamespace)).To(Succeed())
	})

	It("GetResource gets a resource", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
		}

		Expect(c.Create(context.TODO(), ns)).To(Succeed())
		currentNamespace := &corev1.Namespace{}
		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.GetResource(context.TODO(), types.NamespacedName{Name: ns.Name},
			currentNamespace)).To(Succeed())
		Expect(currentNamespace.Name).To(Equal(ns.Name))
	})

	It("UpdateResource updates a resource", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
		}

		Expect(c.Create(context.TODO(), ns)).To(Succeed())
		currentNamespace := &corev1.Namespace{}
		Expect(c.Get(context.TODO(), types.NamespacedName{Name: ns.Name},
			currentNamespace)).To(Succeed())
		currentNamespace.Labels = map[string]string{randomString(): randomString()}
		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.UpdateResource(context.TODO(), currentNamespace)).To(Succeed())

		Expect(c.Get(context.TODO(), types.NamespacedName{Name: ns.Name},
			currentNamespace)).To(Succeed())
		Expect(len(currentNamespace.Labels)).To(Equal(1))
	})

	It("UpdateResourceStatus updates a resource", func() {
		replicas := int32(1)
		depl := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      randomString(),
				Namespace: randomString(),
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
			},
		}

		Expect(c.Create(context.TODO(), depl)).To(Succeed())

		currentDepl := &appsv1.Deployment{}
		Expect(c.Get(context.TODO(),
			types.NamespacedName{Name: depl.Name, Namespace: depl.Namespace},
			currentDepl)).To(Succeed())
		currentDepl.Status.AvailableReplicas = replicas
		currentDepl.Status.ReadyReplicas = replicas
		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.UpdateResourceStatus(context.TODO(), currentDepl)).To(Succeed())

		Expect(c.Get(context.TODO(),
			types.NamespacedName{Name: depl.Name, Namespace: depl.Namespace},
			currentDepl)).To(Succeed())
		Expect(currentDepl.Status.AvailableReplicas).To(Equal(replicas))
		Expect(currentDepl.Status.ReadyReplicas).To(Equal(replicas))
	})

	It("DeleteResource deletes a resource", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
		}

		Expect(c.Create(context.TODO(), ns)).To(Succeed())
		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.DeleteResource(context.TODO(), ns)).To(Succeed())
		currentNamespace := &corev1.Namespace{}
		err := c.Get(context.TODO(), types.NamespacedName{Name: ns.Name},
			currentNamespace)
		Expect(err).ToNot(BeNil())
		Expect(apierrors.IsNotFound(err)).To(BeTrue())
	})

	It("ListResources lists resources", func() {
		replicas := int32(1)
		for i := 0; i < 10; i++ {
			depl := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      randomString(),
					Namespace: randomString(),
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: &replicas,
				},
			}
			Expect(c.Create(context.TODO(), depl)).To(Succeed())
		}

		deplList := &appsv1.DeploymentList{}
		k8sAccess := utils.GetK8sAccess(scheme, c)
		Expect(k8sAccess.ListResources(context.TODO(), deplList)).To(Succeed())
		Expect(len(deplList.Items)).To(Equal(10))
	})

	It("updateResourceWithDynamicResourceInterface creates and updates an object", func() {
		universalDeserializer := kubectlscheme.Codecs.UniversalDeserializer()
		request := &unstructured.Unstructured{}
		nsName := randomString()
		_, _, err := universalDeserializer.Decode([]byte(fmt.Sprintf(nsInstanceTemplate, nsName)), nil, request)
		Expect(err).To(BeNil())

		c, err = client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).To(BeNil())

		k8sAccess := utils.GetK8sAccessWithRestConfig(scheme, c, cfg)

		dr, err := k8sAccess.GetDynamicResourceInterface(request)
		Expect(err).To(BeNil())
		Expect(k8sAccess.UpdateResourceWithDynamicResourceInterface(context.TODO(), dr, request, klogr.New())).To(Succeed())

		currentNs := &corev1.Namespace{}
		Expect(k8sAccess.GetResource(context.TODO(), types.NamespacedName{Name: nsName}, currentNs)).To(Succeed())
		currentLabelLength := len(currentNs.Labels)

		_, _, err = universalDeserializer.Decode([]byte(fmt.Sprintf(nsInstanceTemplateWithLabels, nsName)), nil, request)
		Expect(err).To(BeNil())
		Expect(k8sAccess.UpdateResourceWithDynamicResourceInterface(context.TODO(), dr, request, klogr.New())).To(Succeed())

		Expect(k8sAccess.GetResource(context.TODO(), types.NamespacedName{Name: nsName}, currentNs)).To(Succeed())
		Expect(len(currentNs.Labels)).To(Equal(currentLabelLength + 1))
	})
})
