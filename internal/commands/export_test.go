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

package commands

import (
	utilsv1beta1 "github.com/projectsveltos/sveltosctl/api/v1beta1"
)

var (
	GetNextScheduleTime = getNextScheduleTime
	ShouldSchedule      = shouldSchedule
)

func NewCollectionSnapshot(s *utilsv1beta1.Snapshot) collection {
	return &collectionSnapshot{snapshotInstance: s}
}
