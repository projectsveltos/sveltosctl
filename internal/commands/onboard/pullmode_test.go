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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

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
		utils.InitalizeManagementClusterAcces(scheme, &rest.Config{Host: "https://127.0.0.1:6443"}, nil, c)

		Expect(onboard.OnboardSveltosClusterInPullMode(context.TODO(), clusterNamespace, clusterName, "",
			nil, textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))).To(Succeed())

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
})
