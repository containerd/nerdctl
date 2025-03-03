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

// Command is the simplest way to express a test.TestableCommand for very basic cases
// where access to test data is not necessary.
func Command(args ...string) Executor {
	return func(_ Data, helpers Helpers) TestableCommand {
		return helpers.Command(args...)
	}
}

// Expects is provided as a simple helper covering "expectations" for simple use-cases
// where access to the test data is not necessary.
func Expects(exitCode int, errors []error, output Comparator) Manager {
	return func(_ Data, _ Helpers) *Expected {
		return &Expected{
			ExitCode: exitCode,
			Errors:   errors,
			Output:   output,
		}
	}
}
