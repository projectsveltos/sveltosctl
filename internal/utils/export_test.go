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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var AddToScheme = addToScheme

const DefaultInstanceName = defaultInstanceName

func GetK8sAccess(scheme *runtime.Scheme, c client.Client) *k8sAccess {
	return &k8sAccess{
		scheme:     scheme,
		client:     c,
		clientset:  nil,
		restConfig: nil,
	}
}

func GetK8sAccessWithRestConfig(scheme *runtime.Scheme, c client.Client, restConfig *rest.Config) *k8sAccess {
	return &k8sAccess{
		scheme:     scheme,
		client:     c,
		clientset:  nil,
		restConfig: restConfig,
	}
}
