/*
Copyright 2025. projectsveltos.io. All rights reserved.

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

package onboard_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/klog/v2/textlogger"

	"github.com/projectsveltos/sveltosctl/internal/commands/onboard"
)

var _ = Describe("Register cluster in pullmode", func() {
	It("prepareApplierYAML returns the YAML to apply to managed cluster", func() {
		clusterNamespace := randomString()
		clusterName := randomString()
		kubeconfig := randomString()

		toApply, err := onboard.PrepareApplierYAML(clusterNamespace, clusterName, kubeconfig,
			textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1))))
		Expect(err).To(BeNil())
		Expect(strings.Contains(toApply, fmt.Sprintf("--cluster-namespace=%s", clusterNamespace)))
		Expect(strings.Contains(toApply, fmt.Sprintf("--cluster-name=%s", clusterName)))
		Expect(strings.Contains(toApply, "--cluster-namespace=sveltos"))
		Expect(strings.Contains(toApply, fmt.Sprintf("--secret-with-kubeconfig=%s-sveltos-kubeconfig", clusterName)))
	})
})
