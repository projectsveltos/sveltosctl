/*
Copyright 2026. projectsveltos.io. All rights reserved.

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
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/docopt/docopt-go"
	"github.com/go-logr/logr"
	"github.com/olekukonko/tablewriter"

	libsveltosv1beta1 "github.com/projectsveltos/libsveltos/api/v1beta1"
	logs "github.com/projectsveltos/libsveltos/lib/logsettings"
	"github.com/projectsveltos/sveltosctl/internal/utils"
)

const (
	classifierType = "Classifier"
	mccType        = "ManagementClusterClassifier"
)

// labelValues maps a label key to its declared value for a given classifier instance.
type labelValues map[string]string

func buildClassifierLabelMap(ctx context.Context, logger logr.Logger) (map[string]labelValues, error) {
	instance := utils.GetAccessInstance()
	classifiers, err := instance.ListClassifiers(ctx, logger)
	if err != nil {
		return nil, err
	}
	m := make(map[string]labelValues, len(classifiers.Items))
	for i := range classifiers.Items {
		c := &classifiers.Items[i]
		lv := make(labelValues, len(c.Spec.ClassifierLabels))
		for _, cl := range c.Spec.ClassifierLabels {
			lv[cl.Key] = cl.Value
		}
		m[c.Name] = lv
	}
	return m, nil
}

func buildMCCLabelMap(ctx context.Context, logger logr.Logger) (map[string]labelValues, error) {
	instance := utils.GetAccessInstance()
	mccs, err := instance.ListManagementClusterClassifiers(ctx, logger)
	if err != nil {
		return nil, err
	}
	m := make(map[string]labelValues, len(mccs.Items))
	for i := range mccs.Items {
		mcc := &mccs.Items[i]
		lv := make(labelValues, len(mcc.Spec.ClassifierLabels))
		for _, cl := range mcc.Spec.ClassifierLabels {
			lv[cl.Key] = cl.Value
		}
		m[mcc.Name] = lv
	}
	return m, nil
}

func displayClassifierLabels(ctx context.Context, passedNamespace, passedCluster string,
	warningsOnly bool, logger logr.Logger) error {

	if warningsOnly {
		return displayConflicts(ctx, passedNamespace, passedCluster, logger)
	}

	classifierMap, err := buildClassifierLabelMap(ctx, logger)
	if err != nil {
		return err
	}
	mccMap, err := buildMCCLabelMap(ctx, logger)
	if err != nil {
		return err
	}
	return displayManagedLabels(ctx, passedNamespace, passedCluster, classifierMap, mccMap, logger)
}

func displayManagedLabels(ctx context.Context, passedNamespace, passedCluster string,
	classifierMap, mccMap map[string]labelValues, logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "KEY", "VALUE", "CLASSIFIER/MCC", "TYPE")

	instance := utils.GetAccessInstance()

	reports, err := instance.ListClassifierReports(ctx, passedNamespace, logger)
	if err != nil {
		return err
	}
	for i := range reports.Items {
		r := &reports.Items[i]
		if !matchesCluster(r.Spec.ClusterNamespace, r.Spec.ClusterName, passedNamespace, passedCluster) {
			continue
		}
		clusterInfo := fmt.Sprintf("%s/%s", r.Spec.ClusterNamespace, r.Spec.ClusterName)
		lv := classifierMap[r.Spec.ClassifierName]
		for _, key := range r.Status.ManagedLabels {
			value := lv[key]
			if err := table.Append([]string{clusterInfo, key, value, r.Spec.ClassifierName, classifierType}); err != nil {
				return err
			}
		}
	}

	mccReports, err := instance.ListManagementClusterClassifierReports(ctx, passedNamespace, logger)
	if err != nil {
		return err
	}
	for i := range mccReports.Items {
		r := &mccReports.Items[i]
		if !matchesCluster(r.Spec.ClusterNamespace, r.Spec.ClusterName, passedNamespace, passedCluster) {
			continue
		}
		clusterInfo := fmt.Sprintf("%s/%s", r.Spec.ClusterNamespace, r.Spec.ClusterName)
		lv := mccMap[r.Spec.ClassifierName]
		for _, key := range r.Status.ManagedLabels {
			value := lv[key]
			if err := table.Append([]string{clusterInfo, key, value, r.Spec.ClassifierName, mccType}); err != nil {
				return err
			}
		}
	}

	return table.Render()
}

func displayConflicts(ctx context.Context, passedNamespace, passedCluster string,
	logger logr.Logger) error {

	table := tablewriter.NewWriter(os.Stdout)
	table.Header("CLUSTER", "KEY", "WANTED BY", "TYPE", "CONFLICT")

	instance := utils.GetAccessInstance()

	reports, err := instance.ListClassifierReports(ctx, passedNamespace, logger)
	if err != nil {
		return err
	}
	for i := range reports.Items {
		r := &reports.Items[i]
		if !matchesCluster(r.Spec.ClusterNamespace, r.Spec.ClusterName, passedNamespace, passedCluster) {
			continue
		}
		if err := appendConflictRows(table, r.Spec.ClusterNamespace, r.Spec.ClusterName,
			r.Spec.ClassifierName, classifierType, r.Status.UnManagedLabels); err != nil {
			return err
		}
	}

	mccReports, err := instance.ListManagementClusterClassifierReports(ctx, passedNamespace, logger)
	if err != nil {
		return err
	}
	for i := range mccReports.Items {
		r := &mccReports.Items[i]
		if !matchesCluster(r.Spec.ClusterNamespace, r.Spec.ClusterName, passedNamespace, passedCluster) {
			continue
		}
		if err := appendConflictRows(table, r.Spec.ClusterNamespace, r.Spec.ClusterName,
			r.Spec.ClassifierName, mccType, r.Status.UnManagedLabels); err != nil {
			return err
		}
	}

	return table.Render()
}

func appendConflictRows(table *tablewriter.Table, clusterNamespace, clusterName,
	classifierName, cType string, unmanaged []libsveltosv1beta1.UnManagedLabel) error {

	clusterInfo := fmt.Sprintf("%s/%s", clusterNamespace, clusterName)
	for _, u := range unmanaged {
		msg := ""
		if u.FailureMessage != nil {
			msg = *u.FailureMessage
		}
		if err := table.Append([]string{clusterInfo, u.Key, classifierName, cType, msg}); err != nil {
			return err
		}
	}
	return nil
}

func matchesCluster(clusterNamespace, clusterName, passedNamespace, passedCluster string) bool {
	if passedNamespace != "" && clusterNamespace != passedNamespace {
		return false
	}
	if passedCluster != "" && clusterName != passedCluster {
		return false
	}
	return true
}

// ClassifierLabels displays labels managed by Classifier and ManagementClusterClassifier instances.
func ClassifierLabels(ctx context.Context, args []string, logger logr.Logger) error {
	doc := `Usage:
  sveltosctl show classifier-labels [options] [--namespace=<name>] [--cluster=<name>] [--warnings] [--verbose]

     --namespace=<name>      Show labels for clusters in this namespace.
                             If not specified all namespaces are considered.
     --cluster=<name>        Show labels for the cluster with this name.
                             If not specified all clusters are considered.
     --warnings              Show only label conflicts instead of all managed labels.

Options:
  -h --help                  Show this screen.
     --verbose               Verbose mode. Print each step.

Description:
  The show classifier-labels command shows labels that Classifier and ManagementClusterClassifier
  instances are managing on each cluster. Use --warnings to list only conflicts, where two
  classifiers are competing to own the same label key on the same cluster.
`
	parsedArgs, err := docopt.ParseArgs(doc, nil, "1.0")
	if err != nil {
		logger.V(logs.LogInfo).Error(err, "failed to parse args")
		return fmt.Errorf(
			"invalid option: 'sveltosctl %s'. Use flag '--help' to read about a specific subcommand. Error: %w",
			strings.Join(args, " "),
			err,
		)
	}
	if len(parsedArgs) == 0 {
		return nil
	}

	_ = flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogInfo))
	verbose := parsedArgs["--verbose"].(bool)
	if verbose {
		if err := flag.Lookup("v").Value.Set(fmt.Sprint(logs.LogDebug)); err != nil {
			return err
		}
	}

	namespace := ""
	if passedNamespace := parsedArgs["--namespace"]; passedNamespace != nil {
		namespace = passedNamespace.(string)
	}

	cluster := ""
	if passedCluster := parsedArgs["--cluster"]; passedCluster != nil {
		cluster = passedCluster.(string)
	}

	warningsOnly := parsedArgs["--warnings"].(bool)

	return displayClassifierLabels(ctx, namespace, cluster, warningsOnly, logger)
}
