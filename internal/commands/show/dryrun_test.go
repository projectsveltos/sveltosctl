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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	configv1alpha1 "github.com/projectsveltos/addon-controller/api/v1alpha1"
	"github.com/projectsveltos/addon-controller/controllers"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("DryRun", func() {
	var ns *corev1.Namespace

	BeforeEach(func() {
		namespace := namePrefix + randomString()

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}
	})

	It("show dryrun displays possible changes in helm charts", func() {
		clusterNamespace := ns.Name
		clusterName := randomString()

		releaseReports1 := []configv1alpha1.ReleaseReport{
			*generateReleaseReport(string(configv1alpha1.HelmChartActionInstall)),
			*generateReleaseReport(string(configv1alpha1.NoHelmAction)),
		}

		clusterProfileName1 := randomString()
		clusterReport1 := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      randomString(),
				Labels: map[string]string{
					controllers.ClusterProfileLabelName: clusterProfileName1,
				},
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
			},
			Status: configv1alpha1.ClusterReportStatus{
				ReleaseReports: releaseReports1,
			},
		}

		releaseReports2 := []configv1alpha1.ReleaseReport{
			*generateReleaseReport(string(configv1alpha1.HelmChartActionInstall)),
			*generateReleaseReport(string(configv1alpha1.HelmChartActionUninstall)),
			*generateReleaseReport(string(configv1alpha1.NoHelmAction)),
		}

		clusterProfileName2 := randomString()
		clusterReport2 := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      randomString(),
				Labels: map[string]string{
					controllers.ClusterProfileLabelName: clusterProfileName2,
				},
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
			},
			Status: configv1alpha1.ClusterReportStatus{
				ReleaseReports: releaseReports2,
			},
		}

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{ns, clusterReport1, clusterReport2}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayDryRun(context.TODO(), "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			   +------------------+---------------+-----------+----------------+-----------+--------------------------------+------------------+
			   |      CLUSTER     | RESOURCE TYPE | NAMESPACE |      NAME      |  ACTION   |            MESSAGE             | CLUSTER PROFILES |
			   +------------------+---------------+-----------+----------------+-----------+--------------------------------+------------------+
			   | default/workload | helm release  | kyverno   | kyverno-latest | No Action | Already managing this helm     | cf1              |
			   |                  |               |           |                |           | release and specified version  |                  |
			   |                  |               |           |                |           | already installed              |                  |
			   | default/workload | helm release  | nginx     | nginx-latest   | No Action | Already managing this helm     | cf1              |
			   |                  |               |           |                |           | release and specified version  |                  |
			   |                  |               |           |                |           | already installed              |                  |
			   | default/workload | helm release  | mysql     | mysql          | Install   |                                | cf1              |
			   +------------------+---------------+-----------+----------------+-----------+--------------------------------+------------------+
		*/

		clusterInfo := fmt.Sprintf("%s/%s", clusterNamespace, clusterName)

		lines := strings.Split(buf.String(), "\n")
		verifyReleaseReports(lines, clusterInfo, clusterProfileName1, releaseReports1)
		verifyReleaseReports(lines, clusterInfo, clusterProfileName2, releaseReports2)

		os.Stdout = old
	})

	It("show dryrun displays possible changes in resources", func() {
		clusterNamespace := ns.Name
		clusterName := randomString()

		resourceReports1 := []configv1alpha1.ResourceReport{
			*generateResourceReport(string(configv1alpha1.HelmChartActionInstall)),
			*generateResourceReport(string(configv1alpha1.NoHelmAction)),
		}

		clusterProfileName1 := randomString()
		clusterReport1 := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      randomString(),
				Labels: map[string]string{
					controllers.ClusterProfileLabelName: clusterProfileName1,
				},
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
			},
			Status: configv1alpha1.ClusterReportStatus{
				ResourceReports: resourceReports1,
			},
		}

		resourceReports2 := []configv1alpha1.ResourceReport{
			*generateResourceReport(string(configv1alpha1.CreateResourceAction)),
			*generateResourceReport(string(configv1alpha1.UpdateResourceAction)),
			*generateResourceReport(string(configv1alpha1.NoResourceAction)),
			*generateResourceReport(string(configv1alpha1.DeleteResourceAction)),
		}

		clusterProfileName2 := randomString()
		clusterReport2 := &configv1alpha1.ClusterReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns.Name,
				Name:      randomString(),
				Labels: map[string]string{
					controllers.ClusterProfileLabelName: clusterProfileName2,
				},
			},
			Spec: configv1alpha1.ClusterReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
			},
			Status: configv1alpha1.ClusterReportStatus{
				ResourceReports: resourceReports2,
			},
		}

		old := os.Stdout // keep backup of the real stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		initObjects := []client.Object{ns, clusterReport1, clusterReport2}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()

		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
		err = show.DisplayDryRun(context.TODO(), "", "", "",
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())

		/*
			// This is an example of how the table needs to look like
			   +------------------+---------------+-----------+----------------+-----------+-------------+------------------+
			   |      CLUSTER     | RESOURCE TYPE | NAMESPACE |      NAME      |  ACTION   |    MESSAGE  | CLUSTER PROFILES |
			   +------------------+---------------+-----------+----------------+-----------+-------------+------------------+
			   | default/workload | :Pod          | default   |     nginx      | Create    |             |     cf1          |
			   +----------------------------------+-----------+----------------+-----------+-------------+------------------+
		*/

		clusterInfo := fmt.Sprintf("%s/%s", clusterNamespace, clusterName)

		lines := strings.Split(buf.String(), "\n")
		verifyResourceReports(lines, clusterInfo, clusterProfileName1, resourceReports1)
		verifyResourceReports(lines, clusterInfo, clusterProfileName2, resourceReports2)

		os.Stdout = old
	})
})

func verifyReleaseReports(lines []string, clusterInfo, clusterProfileName string,
	releaseReports []configv1alpha1.ReleaseReport) {

	for i := range releaseReports {
		verifyReleaseReport(lines, clusterInfo, clusterProfileName, &releaseReports[i])
	}
}

func verifyReleaseReport(lines []string, clusterInfo, clusterProfileName string,
	releaseReport *configv1alpha1.ReleaseReport) {

	found := false
	for i := range lines {
		if strings.Contains(lines[i], clusterInfo) &&
			strings.Contains(lines[i], clusterProfileName) &&
			strings.Contains(lines[i], releaseReport.ReleaseNamespace) &&
			strings.Contains(lines[i], releaseReport.ReleaseName) &&
			strings.Contains(lines[i], releaseReport.Action) {

			found = true
			break
		}
	}
	if found != true {
		By(fmt.Sprintf("Failed to verify release report %s/%s",
			releaseReport.ReleaseNamespace, releaseReport.ReleaseName))
		By(fmt.Sprintf("Results: %v", lines))
	}
	Expect(found).To(BeTrue())
}

func verifyResourceReports(lines []string, clusterInfo, clusterProfileName string,
	resourceReports []configv1alpha1.ResourceReport) {

	for i := range resourceReports {
		verifyResourceReport(lines, clusterInfo, clusterProfileName, &resourceReports[i])
	}
}

func verifyResourceReport(lines []string, clusterInfo, clusterProfileName string,
	resourceReport *configv1alpha1.ResourceReport) {

	found := false
	for i := range lines {
		if strings.Contains(lines[i], clusterInfo) &&
			strings.Contains(lines[i], clusterProfileName) &&
			strings.Contains(lines[i], resourceReport.Resource.Namespace) &&
			strings.Contains(lines[i], resourceReport.Resource.Name) &&
			strings.Contains(lines[i], resourceReport.Resource.Group) &&
			strings.Contains(lines[i], resourceReport.Resource.Kind) &&
			strings.Contains(lines[i], resourceReport.Action) {

			found = true
			break
		}
	}
	if found != true {
		By(fmt.Sprintf("Failed to verify resource report %s/%s",
			resourceReport.Resource.Namespace, resourceReport.Resource.Name))
		By(fmt.Sprintf("Results: %v", lines))
	}
	Expect(found).To(BeTrue())
}
