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

package show_test

import (
	"bytes"
	"context"
	"io"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands/show"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Classifier Labels", func() {
	var classifier *libsveltosv1beta1.Classifier
	var mcc *libsveltosv1beta1.ManagementClusterClassifier
	var clusterNamespace, clusterName string

	BeforeEach(func() {
		clusterNamespace = randomString()
		clusterName = randomString()

		classifier = &libsveltosv1beta1.Classifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1beta1.ClassifierSpec{
				ClassifierLabels: []libsveltosv1beta1.ClassifierLabel{
					{Key: "env", Value: "production"},
				},
			},
		}

		mcc = &libsveltosv1beta1.ManagementClusterClassifier{
			ObjectMeta: metav1.ObjectMeta{
				Name: randomString(),
			},
			Spec: libsveltosv1beta1.ManagementClusterClassifierSpec{
				ClassifierLabels: []libsveltosv1beta1.ClassifierLabel{
					{Key: "region", Value: "us-east"},
				},
			},
		}
	})

	It("show classifier-labels displays labels managed by Classifier and ManagementClusterClassifier", func() {
		classifierReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: randomString(),
				Name:      randomString(),
			},
			Spec: libsveltosv1beta1.ClassifierReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      libsveltosv1beta1.ClusterTypeCapi,
				ClassifierName:   classifier.Name,
				Match:            true,
			},
			Status: libsveltosv1beta1.ClassifierReportStatus{
				ManagedLabels: []string{"env"},
			},
		}

		mccReport := &libsveltosv1beta1.ManagementClusterClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: randomString(),
				Name:      randomString(),
			},
			Spec: libsveltosv1beta1.ManagementClusterClassifierReportSpec{
				ClassifierName:   mcc.Name,
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      libsveltosv1beta1.ClusterTypeSveltos,
			},
			Status: libsveltosv1beta1.ManagementClusterClassifierReportStatus{
				ManagedLabels: []string{"region"},
			},
		}

		initObjects := []client.Object{classifier, mcc, classifierReport, mccReport}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayClassifierLabels(context.TODO(), "", "", false,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())
		os.Stdout = old

		output := buf.String()
		Expect(output).To(ContainSubstring(clusterNamespace + "/" + clusterName))
		Expect(output).To(ContainSubstring("env"))
		Expect(output).To(ContainSubstring("production"))
		Expect(output).To(ContainSubstring(classifier.Name))
		Expect(output).To(ContainSubstring("region"))
		Expect(output).To(ContainSubstring("us-east"))
		Expect(output).To(ContainSubstring(mcc.Name))
	})

	It("show classifier-labels filters by namespace and cluster", func() {
		classifierReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      randomString(),
			},
			Spec: libsveltosv1beta1.ClassifierReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      libsveltosv1beta1.ClusterTypeCapi,
				ClassifierName:   classifier.Name,
				Match:            true,
			},
			Status: libsveltosv1beta1.ClassifierReportStatus{
				ManagedLabels: []string{"env"},
			},
		}

		otherNamespace := randomString()
		otherCluster := randomString()
		otherReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: otherNamespace,
				Name:      randomString(),
			},
			Spec: libsveltosv1beta1.ClassifierReportSpec{
				ClusterNamespace: otherNamespace,
				ClusterName:      otherCluster,
				ClusterType:      libsveltosv1beta1.ClusterTypeCapi,
				ClassifierName:   classifier.Name,
				Match:            true,
			},
			Status: libsveltosv1beta1.ClassifierReportStatus{
				ManagedLabels: []string{"env"},
			},
		}

		initObjects := []client.Object{classifier, classifierReport, otherReport}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayClassifierLabels(context.TODO(), clusterNamespace, clusterName, false,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())
		os.Stdout = old

		output := buf.String()
		Expect(output).To(ContainSubstring(clusterNamespace + "/" + clusterName))
		Expect(output).NotTo(ContainSubstring(otherNamespace + "/" + otherCluster))
	})

	It("show classifier-labels --warnings displays only label conflicts", func() {
		failureMessage := randomString()
		classifierReport := &libsveltosv1beta1.ClassifierReport{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: randomString(),
				Name:      randomString(),
			},
			Spec: libsveltosv1beta1.ClassifierReportSpec{
				ClusterNamespace: clusterNamespace,
				ClusterName:      clusterName,
				ClusterType:      libsveltosv1beta1.ClusterTypeCapi,
				ClassifierName:   classifier.Name,
				Match:            true,
			},
			Status: libsveltosv1beta1.ClassifierReportStatus{
				ManagedLabels: []string{"env"},
				UnManagedLabels: []libsveltosv1beta1.UnManagedLabel{
					{Key: "region", FailureMessage: &failureMessage},
				},
			},
		}

		initObjects := []client.Object{classifier, classifierReport}

		scheme, err := utils.GetScheme()
		Expect(err).To(BeNil())
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(initObjects...).Build()
		utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)

		old := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err = show.DisplayClassifierLabels(context.TODO(), "", "", true,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())

		w.Close()
		var buf bytes.Buffer
		_, err = io.Copy(&buf, r)
		Expect(err).To(BeNil())
		os.Stdout = old

		output := buf.String()
		Expect(output).To(ContainSubstring("region"))
		Expect(output).To(ContainSubstring(failureMessage))
		Expect(output).To(ContainSubstring(classifier.Name))
		Expect(output).NotTo(ContainSubstring("production"))
	})
})
