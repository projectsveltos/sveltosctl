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

package utils

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
)

// ListHealthCheckReports returns all current HealthCheckReports
func (a *k8sAccess) ListHealthCheckReports(ctx context.Context, namespace string,
	logger logr.Logger) (*libsveltosv1beta1.HealthCheckReportList, error) {

	logger.V(logs.LogDebug).Info("Get all HealthCheckReports")

	listOptions := []client.ListOption{}
	if namespace != "" {
		listOptions = []client.ListOption{
			client.InNamespace(namespace),
		}
	}

	healthCheckReports := &libsveltosv1beta1.HealthCheckReportList{}
	err := a.client.List(ctx, healthCheckReports, listOptions...)
	return healthCheckReports, err
}
