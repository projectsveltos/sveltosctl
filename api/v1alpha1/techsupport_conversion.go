/*
Copyright 2024. projectsveltos.io. All rights reserved.

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

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apimachineryconversion "k8s.io/apimachinery/pkg/conversion"
	"sigs.k8s.io/controller-runtime/pkg/conversion"

	libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
)

// ConvertTo converts v1alpha1 to the Hub version (v1beta1).
func (src *Techsupport) ConvertTo(dstRaw conversion.Hub) error {
	dst := dstRaw.(*utilsv1beta1.Techsupport)
	err := Convert_v1alpha1_Techsupport_To_v1beta1_Techsupport(src, dst, nil)
	if err != nil {
		return err
	}

	if src.Spec.ClusterSelector == "" {
		dst.Spec.ClusterSelector.LabelSelector = metav1.LabelSelector{}
	}

	return nil
}

// ConvertFrom converts from the Hub version (v1beta1) to this v1alpha1.
func (dst *Techsupport) ConvertFrom(srcRaw conversion.Hub) error {
	src := srcRaw.(*utilsv1beta1.Techsupport)
	err := Convert_v1beta1_Techsupport_To_v1alpha1_Techsupport(src, dst, nil)
	if err != nil {
		return err
	}

	if src.Spec.ClusterSelector.MatchLabels == nil {
		dst.Spec.ClusterSelector = ""
	}

	return nil
}

func Convert_v1alpha1_TechsupportSpec_To_v1beta1_TechsupportSpec(srcSpec *TechsupportSpec, dstSpec *utilsv1beta1.TechsupportSpec,
	scope apimachineryconversion.Scope) error {

	if err := autoConvert_v1alpha1_TechsupportSpec_To_v1beta1_TechsupportSpec(srcSpec, dstSpec, nil); err != nil {
		return err
	}

	labelSelector, err := metav1.ParseToLabelSelector(string(srcSpec.ClusterSelector))
	if err != nil {
		return fmt.Errorf("error converting labels.Selector to metav1.Selector: %w", err)
	}
	dstSpec.ClusterSelector = libsveltosv1beta1.Selector{LabelSelector: *labelSelector}

	return nil
}

func Convert_v1beta1_TechsupportSpec_To_v1alpha1_TechsupportSpec(srcSpec *utilsv1beta1.TechsupportSpec, dstSpec *TechsupportSpec,
	scope apimachineryconversion.Scope) error {

	if err := autoConvert_v1beta1_TechsupportSpec_To_v1alpha1_TechsupportSpec(srcSpec, dstSpec, nil); err != nil {
		return err
	}

	labelSelector, err := srcSpec.ClusterSelector.ToSelector()
	if err != nil {
		return fmt.Errorf("failed to convert : %w", err)
	}

	dstSpec.ClusterSelector = libsveltosv1alpha1.Selector(labelSelector.String())

	return nil
}
