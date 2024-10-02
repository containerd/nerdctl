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

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

// RunCommand is the simplest way to express a test.Command for very basic cases when access to test data is not necessary
func RunCommand(args ...string) Executor {
	return func(data Data, helpers Helpers) Command {
		return helpers.Command(args...)
	}
}

// Expects is provided as a simple helper covering "expectations" for simple use-cases where access to the test data is not necessary
func Expects(exitCode int, errors []error, output Comparator) Manager {
	return func(_ Data, _ Helpers) *Expected {
		return &Expected{
			ExitCode: exitCode,
			Errors:   errors,
			Output:   output,
		}
	}
}

// WithData returns a data object with a certain key value set
func WithData(key string, value string) Data {
	dat := &data{}
	dat.Set(key, value)
	return dat
}

// WithConfig returns a data object with a certain config property set
func WithConfig(key ConfigKey, value ConfigValue) Data {
	dat := &data{}
	dat.WithConfig(key, value)
	return dat
}

// All can be used as a parameter for expected.Output to group a set of comparators
func All(comparators ...Comparator) Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		for _, comparator := range comparators {
			comparator(stdout, info, t)
		}
	}
}

// Contains can be used as a parameter for expected.Output and ensures a comparison string is found contained in the output
func Contains(compare string) Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, strings.Contains(stdout, compare), fmt.Sprintf("Output does not contain: %q", compare)+info)
	}
}

// DoesNotContain is to be used for expected.Output to ensure a comparison string is NOT found in the output
func DoesNotContain(compare string) Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, !strings.Contains(stdout, compare), fmt.Sprintf("Output does contain: %q", compare)+info)
	}
}

// Equals is to be used for expected.Output to ensure it is exactly the output
func Equals(compare string) Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Equal(t, compare, stdout, info)
	}
}

// Provisional - expected use, but have not seen it so far
// Match is to be used for expected.Output to ensure we match a regexp
func Match(reg *regexp.Regexp) Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, reg.MatchString(stdout), fmt.Sprintf("Output does not match: %s", reg), info)
	}
}
