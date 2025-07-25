//go:build ignore
// +build ignore

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

package main

import (
	"os"
	"path/filepath"
	"text/template"
)

const (
	agentTemplate = `// Generated by *go generate* - DO NOT EDIT
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
package agent

var {{ .ExportedVar }}YAML = []byte({{- printf "%s" .YAML -}})
`
)

func generate(filename, outputFilename, manifest string) {
	// Get classifier-agent file
	fileAbs, err := filepath.Abs(filename)
	if err != nil {
		panic(err)
	}

	content, err := os.ReadFile(fileAbs)
	if err != nil {
		panic(err)
	}
	contentStr := "`" + string(content) + "`"

	// Find the output.
	agent, err := os.Create(outputFilename + ".go")
	if err != nil {
		panic(err)
	}
	defer agent.Close()

	// Store file contents.
	type Info struct {
		YAML        string
		File        string
		ExportedVar string
	}
	mi := Info{
		YAML:        contentStr,
		File:        filename,
		ExportedVar: manifest,
	}

	// Generate template.
	manifesTemplate := template.Must(template.New("sveltos-applier-generate").Parse(agentTemplate))
	if err := manifesTemplate.Execute(agent, mi); err != nil {
		panic(err)
	}
}

func main() {
	sveltosAgentFile := "../../internal/agent/sveltos-applier.yaml"
	generate(sveltosAgentFile, "sveltos-applier", "sveltosApplier")
}
