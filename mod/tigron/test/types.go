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

type (
	// ConfigKey FIXME consider getting rid of this?
	ConfigKey string
	// ConfigValue FIXME consider getting rid of this?
	ConfigValue string
)

// A Requirement offers a way to verify random conditions to decide if a test should be skipped or run.
// It can also (optionally) provide custom Setup and Cleanup routines.
type Requirement struct {
	// Check is expected to verify if the requirement is fulfilled, and return a boolean and an
	// explanatory message.
	Check Evaluator
	// Setup, if provided, will be run before any test-specific Setup routine, in the order that
	// requirements have been declared.
	Setup Butler
	// Cleanup, if provided, will be run after any test-specific Cleanup routine, in the reverse
	// order that requirements have been declared.
	Cleanup Butler
}

// Expected expresses the expected output of a command.
type Expected struct {
	// ExitCode.
	ExitCode int
	// Errors contains any error that (once serialized) should be seen in stderr.
	Errors []error
	// Output function to match against stdout.
	Output Comparator
}
