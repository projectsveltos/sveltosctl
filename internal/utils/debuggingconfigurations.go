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

package utils

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
)

const (
	defaultInstanceName = "default"
)

// GetDebuggingConfiguration gets default DebuggingConfiguration
func (a *k8sAccess) GetDebuggingConfiguration(
	ctx context.Context,
) (*libsveltosv1beta1.DebuggingConfiguration, error) {

	req := &libsveltosv1beta1.DebuggingConfiguration{}

	reqName := client.ObjectKey{
		Name: defaultInstanceName,
	}

	if err := a.client.Get(ctx, reqName, req); err != nil {
		return nil, err
	}

	return req, nil
}

// UpdateDebuggingConfiguration creates, if not existing already, default DebuggingConfiguration. Otherwise
// updates it.
func (a *k8sAccess) UpdateDebuggingConfiguration(
	ctx context.Context,
	dc *libsveltosv1beta1.DebuggingConfiguration,
) error {

	reqName := client.ObjectKey{
		Name: defaultInstanceName,
	}

	tmp := &libsveltosv1beta1.DebuggingConfiguration{}

	err := a.client.Get(ctx, reqName, tmp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = a.client.Create(ctx, dc)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	err = a.client.Update(ctx, dc)
	if err != nil {
		return err
	}

	return nil
}
