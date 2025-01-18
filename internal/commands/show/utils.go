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

package show

import (
	corev1 "k8s.io/api/core/v1"

	configv1beta1 "github.com/projectsveltos/addon-controller/api/v1beta1"
)

func doConsiderNamespace(ns *corev1.Namespace, passedNamespace string) bool {
	if passedNamespace == "" {
		return true
	}

	return ns.Name == passedNamespace
}

func doConsiderClusterConfiguration(clusterConfiguration *configv1beta1.ClusterConfiguration,
	passedCluster string) bool {

	if passedCluster == "" {
		return true
	}

	if clusterConfiguration.Labels == nil {
		return false
	}

	clusterName := clusterConfiguration.Labels[configv1beta1.ClusterNameLabel]
	return clusterName == passedCluster
}

func doConsiderClusterReport(clusterReport *configv1beta1.ClusterReport,
	passedCluster string) bool {

	if passedCluster == "" {
		return true
	}

	return clusterReport.Spec.ClusterName == passedCluster
}

func doConsiderProfile(profileNames []string, passedProfile string) bool {
	if passedProfile == "" {
		return true
	}

	for i := range profileNames {
		if profileNames[i] == passedProfile {
			return true
		}
	}

	return false
}
