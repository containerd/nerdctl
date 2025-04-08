/*
   Copyright The containerd Authors.

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

package test

import "testing"

// An Evaluator is a function that decides whether a test should run or not.
type Evaluator func(data Data, helpers Helpers) (bool, string)

// A Butler is the function signature meant to be attached to a Setup or Cleanup routine for a Case
// or Requirement.
type Butler func(data Data, helpers Helpers)

// TODO: when we will break API:
// - remove the info parameter
// - move to tig.T

// A Comparator is the function signature to implement for the Output property of an Expected.
type Comparator func(stdout, info string, t *testing.T)

// A Manager is the function signature meant to produce expectations for a command.
type Manager func(data Data, helpers Helpers) *Expected

// An Executor is the function signature meant to be attached to the Command property of a Case.
type Executor func(data Data, helpers Helpers) TestableCommand
