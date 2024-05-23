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

package loglevel_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "sigs.k8s.io/controller-runtime/pkg/client/fake"

    libsveltosv1alpha1 "github.com/projectsveltos/libsveltos/api/v1alpha1"
    "github.com/projectsveltos/sveltosctl/internal/commands/loglevel"
    "github.com/projectsveltos/sveltosctl/internal/utils"
)

var _ = Describe("Set", func() {
    It("set updates default DebuggingConfiguration instance", func() {
        scheme, err := utils.GetScheme()
        Expect(err).To(BeNil())
        c := fake.NewClientBuilder().WithScheme(scheme).Build()

        utils.InitalizeManagementClusterAcces(scheme, nil, nil, c)
        
        dc := &libsveltosv1alpha1.DebuggingConfiguration{
            ObjectMeta: metav1.ObjectMeta{
                Name: utils.DefaultInstanceName,
            },
        }
        c.Create(context.TODO(), dc)

        Expect(loglevel.UpdateDebuggingConfiguration(context.TODO(), libsveltosv1alpha1.LogLevelDebug,
            string(libsveltosv1alpha1.ComponentAddonManager), "", "", "")).To(Succeed())

        k8sAccess := utils.GetAccessInstance()

        currentDC, err := k8sAccess.GetDebuggingConfiguration(context.TODO(), "", "", "")
        Expect(err).To(BeNil())
        Expect(currentDC).ToNot(BeNil())
        Expect(currentDC.Spec.Configuration).ToNot(BeNil())
        Expect(len(currentDC.Spec.Configuration)).To(Equal(1))
        Expect(currentDC.Spec.Configuration[0].Component).To(Equal(libsveltosv1alpha1.ComponentAddonManager))
        Expect(currentDC.Spec.Configuration[0].LogLevel).To(Equal(libsveltosv1alpha1.LogLevelDebug))

        Expect(loglevel.UpdateDebuggingConfiguration(context.TODO(), libsveltosv1alpha1.LogLevelInfo,
            string(libsveltosv1alpha1.ComponentAddonManager), "", "", "")).To(Succeed())
        currentDC, err = k8sAccess.GetDebuggingConfiguration(context.TODO(), "", "", "")
        Expect(err).To(BeNil())
        Expect(currentDC).ToNot(BeNil())
        Expect(currentDC.Spec.Configuration).ToNot(BeNil())
        Expect(len(currentDC.Spec.Configuration)).To(Equal(1))
        Expect(currentDC.Spec.Configuration[0].Component).To(Equal(libsveltosv1alpha1.ComponentAddonManager))
        Expect(currentDC.Spec.Configuration[0].LogLevel).To(Equal(libsveltosv1alpha1.LogLevelInfo))
    })
})
