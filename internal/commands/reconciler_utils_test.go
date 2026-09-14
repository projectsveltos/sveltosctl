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

package commands_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2/textlogger"

	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
	"github.com/projectsveltos/sveltosctl/internal/commands"
)

var _ = Describe("getNextScheduleTime", func() {
	It("returns the next time after now for a simple schedule", func() {
		now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
		creationTime := metav1.NewTime(now.Add(-time.Hour))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.CreationTimestamp = creationTime
		snapshot.Spec.Schedule = "0 * * * *"

		instance := commands.NewCollectionSnapshot(snapshot)

		next, err := commands.GetNextScheduleTime(instance, now)
		Expect(err).To(BeNil())
		Expect(next).ToNot(BeNil())
		Expect(next.After(now)).To(BeTrue())
	})

	It("returns an error for an invalid schedule", func() {
		now := time.Now()
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.Spec.Schedule = "not-a-valid-schedule"

		instance := commands.NewCollectionSnapshot(snapshot)

		_, err := commands.GetNextScheduleTime(instance, now)
		Expect(err).ToNot(BeNil())
	})

	It("returns an error when too many schedules have been missed", func() {
		now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
		creationTime := metav1.NewTime(now.AddDate(-1, 0, 0))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.CreationTimestamp = creationTime
		snapshot.Spec.Schedule = "* * * * *"

		instance := commands.NewCollectionSnapshot(snapshot)

		_, err := commands.GetNextScheduleTime(instance, now)
		Expect(err).ToNot(BeNil())
	})

	It("clamps the earliest time using startingDeadlineSeconds", func() {
		now := time.Date(2026, time.January, 1, 10, 0, 0, 0, time.UTC)
		creationTime := metav1.NewTime(now.AddDate(-1, 0, 0))
		startingDeadlineSeconds := int64(60)
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.CreationTimestamp = creationTime
		snapshot.Spec.Schedule = "* * * * *"
		snapshot.Spec.StartingDeadlineSeconds = &startingDeadlineSeconds

		instance := commands.NewCollectionSnapshot(snapshot)

		next, err := commands.GetNextScheduleTime(instance, now)
		Expect(err).To(BeNil())
		Expect(next).ToNot(BeNil())
	})
})

var _ = Describe("shouldSchedule", func() {
	logger := textlogger.NewLogger(textlogger.NewConfig(textlogger.Verbosity(1)))

	It("returns false when next schedule time is in the future", func() {
		now := time.Now()
		nextScheduleTime := metav1.NewTime(now.Add(time.Hour))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.Status.NextScheduleTime = &nextScheduleTime

		instance := commands.NewCollectionSnapshot(snapshot)

		Expect(commands.ShouldSchedule(instance, logger)).To(BeFalse())
	})

	It("returns false when last run was less than 30 seconds ago", func() {
		now := time.Now()
		nextScheduleTime := metav1.NewTime(now.Add(-time.Hour))
		lastRunTime := metav1.NewTime(now.Add(-5 * time.Second))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.Status.NextScheduleTime = &nextScheduleTime
		snapshot.Status.LastRunTime = &lastRunTime

		instance := commands.NewCollectionSnapshot(snapshot)

		Expect(commands.ShouldSchedule(instance, logger)).To(BeFalse())
	})

	It("returns true when last run was more than 30 seconds ago", func() {
		now := time.Now()
		nextScheduleTime := metav1.NewTime(now.Add(-time.Hour))
		lastRunTime := metav1.NewTime(now.Add(-time.Minute))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.Status.NextScheduleTime = &nextScheduleTime
		snapshot.Status.LastRunTime = &lastRunTime

		instance := commands.NewCollectionSnapshot(snapshot)

		Expect(commands.ShouldSchedule(instance, logger)).To(BeTrue())
	})

	It("returns true when past next schedule time and never run before", func() {
		now := time.Now()
		nextScheduleTime := metav1.NewTime(now.Add(-time.Hour))
		snapshot := &utilsv1beta1.Snapshot{}
		snapshot.Status.NextScheduleTime = &nextScheduleTime

		instance := commands.NewCollectionSnapshot(snapshot)

		Expect(commands.ShouldSchedule(instance, logger)).To(BeTrue())
	})
})
