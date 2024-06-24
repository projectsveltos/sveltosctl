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

package commands_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands"
)

const (
	upstreamClusterNamePrefix = "predicates-"
)

var _ = Describe("ClusterProfile Predicates: SvelotsClusterPredicates", func() {
	var logger logr.Logger
	var cluster *libsveltosv1beta1.SveltosCluster

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		cluster = &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamClusterNamePrefix + randomString(),
				Namespace: "scpredicates" + randomString(),
			},
		}
	})

	It("Create reprocesses when sveltos Cluster is unpaused", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		result := clusterPredicate.Create(event.TypedCreateEvent[*libsveltosv1beta1.SveltosCluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Create does not reprocess when sveltos Cluster is paused", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Create(event.TypedCreateEvent[*libsveltosv1beta1.SveltosCluster]{Object: cluster})
		Expect(result).To(BeFalse())
	})
	It("Delete does reprocess ", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		result := clusterPredicate.Delete(event.TypedDeleteEvent[*libsveltosv1beta1.SveltosCluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when sveltos Cluster paused changes from true to false", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = true
		oldCluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*libsveltosv1beta1.SveltosCluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when sveltos Cluster paused changes from false to true", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}
		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*libsveltosv1beta1.SveltosCluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update does not reprocess when sveltos Cluster paused has not changed", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false
		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*libsveltosv1beta1.SveltosCluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update reprocesses when sveltos Cluster labels change", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Labels = map[string]string{"department": "eng"}

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
		}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*libsveltosv1beta1.SveltosCluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when sveltos Cluster Status Ready changes", func() {
		clusterPredicate := commands.SveltosClusterPredicate{Logger: logger}

		cluster.Status.Ready = true

		oldCluster := &libsveltosv1beta1.SveltosCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
			Status: libsveltosv1beta1.SveltosClusterStatus{
				Ready: false,
			},
		}
		result := clusterPredicate.Update(event.TypedUpdateEvent[*libsveltosv1beta1.SveltosCluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
})

var _ = Describe("ClusterProfile Predicates: ClusterPredicates", func() {
	var logger logr.Logger
	var cluster *clusterv1.Cluster

	BeforeEach(func() {
		logger = textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))
		cluster = &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      upstreamClusterNamePrefix + randomString(),
				Namespace: "cpredicates" + randomString(),
			},
		}
	})

	It("Create reprocesses when v1Cluster is unpaused", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		result := clusterPredicate.Create(event.TypedCreateEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Create does not reprocess when v1Cluster is paused", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Create(event.TypedCreateEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeFalse())
	})
	It("Delete does reprocess ", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		result := clusterPredicate.Delete(event.TypedDeleteEvent[*clusterv1.Cluster]{Object: cluster})
		Expect(result).To(BeTrue())
	})
	It("Update reprocesses when v1Cluster paused changes from true to false", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false

		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = true
		oldCluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
	It("Update does not reprocess when v1Cluster paused changes from false to true", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = true
		cluster.Annotations = map[string]string{clusterv1.PausedAnnotation: "true"}
		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update does not reprocess when v1Cluster paused has not changed", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Spec.Paused = false
		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
			},
		}
		oldCluster.Spec.Paused = false

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeFalse())
	})
	It("Update reprocesses when v1Cluster labels change", func() {
		clusterPredicate := commands.ClusterPredicate{Logger: logger}

		cluster.Labels = map[string]string{"department": "eng"}

		oldCluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cluster.Name,
				Namespace: cluster.Namespace,
				Labels:    map[string]string{},
			},
		}

		result := clusterPredicate.Update(event.TypedUpdateEvent[*clusterv1.Cluster]{
			ObjectNew: cluster, ObjectOld: oldCluster})
		Expect(result).To(BeTrue())
	})
})
