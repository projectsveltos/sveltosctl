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

package techsupport_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/cluster-api/util"
)

const (
	timeFormat = "2006-01-02:15:04:05"
)

func TestTechsupport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Techsupport Suite")
}

func createTechsupportDirectories(techsupportName, techsupportStorage string, numOfDirs int) string {
	techsupportDir, err := os.MkdirTemp("", randomString())
	Expect(err).To(BeNil())
	techsupportDir = filepath.Join(techsupportDir, techsupportStorage)
	Expect(os.Mkdir(techsupportDir, os.ModePerm)).To(Succeed())
	tmpDir := filepath.Join(techsupportDir, "techsupport")
	Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
	tmpDir = filepath.Join(tmpDir, techsupportName)
	Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())

	now := time.Now()
	for i := 0; i < numOfDirs; i++ {
		timeFolder := now.Add(-time.Second * time.Duration(2*i)).Format(timeFormat)
		tmpDir := filepath.Join(techsupportDir, "techsupport", techsupportName, timeFolder)
		Expect(os.Mkdir(tmpDir, os.ModePerm)).To(Succeed())
		By(fmt.Sprintf("Created temporary directory %s", tmpDir))
	}

	By(fmt.Sprintf("Techsupport directory: %s", techsupportDir))
	return techsupportDir
}

func randomString() string {
	const length = 10
	return util.RandomString(length)
}
