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

package show_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	viewClusterRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
- apiGroups: [""] # "" indicates the core API group
  resources: ["pods"]
  verbs: ["get", "watch", "list"]`

	modifyClusterRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: %s
rules:
- apiGroups: [""] # "" indicates the core API group
  resources: ["namespaces"]
  verbs: ["*"]`

	viewRole = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s
  namespace: default
rules:
- apiGroups: [""] # "" indicates the core API group
  resources: ["pods"]
  verbs: ["get", "watch", "list"]`
)

var _ = Describe("Admin RBACs", func() {
	It("displayAdminRbacs displays per clusters, admin rbacs", func() {
		referecedResourceNamespace := randomString()
		configMapName := randomString()
		viewClusterRoleName := randomString()
		viewRoleName := randomString()
		configMap := createConfigMapWithPolicy(referecedResourceNamespace, configMapName,
			fmt.Sprintf(viewClusterRole, viewClusterRoleName), fmt.Sprintf(viewRole, viewRoleName))

		secretName := randomString()
		modifyClusterRoleName := randomString()
		secret := createSecretWithPolicy(referecedResourceNamespace, secretName, fmt.Sprintf(modifyClusterRole, modifyClusterRoleName))

		clusterNamespace := randomString()
		clusterName := randomString()
		matchingCluster := []corev1.ObjectReference{
			{
				Kind:       libsveltosv1alpha1.SveltosClusterKind,
				APIVersion: libsveltosv1alpha1.GroupVersion.String(),
				Namespace:  clusterNamespace,
				Name:       clusterName,
			},
		}
		roleRequest := getRoleRequest(matchingCluster, []corev1.ConfigMap{*configMap}, []corev1.Secret{*secret},
			randomString(), randomString())

		initObjects := []client.Object{roleRequest, configMap, secret}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayAdminRbacs(context.TODO(), "", "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			Expected
			      +--------------------------------------+-------------+-----------+------------+-----------+----------------+----------------+
			      |               CLUSTER                |    ADMIN    | NAMESPACE | API GROUPS | RESOURCES | RESOURCE NAMES |     VERBS      |
			      +--------------------------------------+-------------+-----------+------------+-----------+----------------+----------------+
			      | SveltosCluster:hjcodszbpx/41bvjygery | 2j9l/l3f66n | default   |            | pods       |                | get,watch,list |
			      | SveltosCluster:hjcodszbpx/41bvjygery | 2j9l/l3f66n | *         |            | pods       |                | get,watch,list |
			      | SveltosCluster:hjcodszbpx/41bvjygery | 2j9l/l3f66n | *         |            | namespaces |                | *              |
			      +--------------------------------------+-------------+-----------+------------+------------+----------------+----------------+
		*/

		clusterInfo := fmt.Sprintf("%s:%s/%s", libsveltosv1alpha1.SveltosClusterKind, clusterNamespace, clusterName)
		lines := strings.Split(buf.String(), "\n")
		clusterRoleView, clusterRoleModify, roleView := false, false, false
		for i := range lines {
			l := lines[i]
			if strings.Contains(l, clusterInfo) && strings.Contains(l, roleRequest.Spec.ServiceAccountNamespace) &&
				strings.Contains(l, roleRequest.Spec.ServiceAccountName) {
				if strings.Contains(l, "default") && strings.Contains(l, "pods") && strings.Contains(l, "get,watch,list") {
					roleView = true
				} else if strings.Contains(l, "*") && strings.Contains(l, "namespaces") {
					clusterRoleModify = true
				} else if strings.Contains(l, "*") && strings.Contains(l, "pods") && strings.Contains(l, "get,watch,list") {
					clusterRoleView = true
				}
			}
		}

		Expect(clusterRoleView).To(BeTrue())
		Expect(clusterRoleModify).To(BeTrue())
		Expect(roleView).To(BeTrue())

		os.Stdout = old
	})
})

func getRoleRequest(matchingClusters []corev1.ObjectReference,
	configMaps []corev1.ConfigMap, secrets []corev1.Secret,
	serviceAccountNamespace, serviceAccountName string) *libsveltosv1alpha1.RoleRequest {

	roleRequest := libsveltosv1alpha1.RoleRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: randomString(),
		},
		Spec: libsveltosv1alpha1.RoleRequestSpec{
			RoleRefs:                make([]libsveltosv1alpha1.PolicyRef, 0),
			ServiceAccountNamespace: serviceAccountNamespace,
			ServiceAccountName:      serviceAccountName,
		},
		Status: libsveltosv1alpha1.RoleRequestStatus{
			MatchingClusterRefs: matchingClusters,
		},
	}

	for i := range configMaps {
		roleRequest.Spec.RoleRefs = append(roleRequest.Spec.RoleRefs, libsveltosv1alpha1.PolicyRef{
			Kind:      string(libsveltosv1alpha1.ConfigMapReferencedResourceKind),
			Namespace: configMaps[i].Namespace,
			Name:      configMaps[i].Name,
		})
	}

	for i := range secrets {
		roleRequest.Spec.RoleRefs = append(roleRequest.Spec.RoleRefs, libsveltosv1alpha1.PolicyRef{
			Kind:      string(libsveltosv1alpha1.SecretReferencedResourceKind),
			Namespace: secrets[i].Namespace,
			Name:      secrets[i].Name,
		})
	}

	return &roleRequest
}
