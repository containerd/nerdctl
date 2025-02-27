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

package expect

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/test"
)

// All can be used as a parameter for expected.Output to group a set of comparators.
func All(comparators ...test.Comparator) test.Comparator {
	//nolint:thelper
	return func(stdout string, info string, t *testing.T) {
		t.Helper()

		for _, comparator := range comparators {
			comparator(stdout, info, t)
		}
	}
}

// Contains can be used as a parameter for expected.Output and ensures a comparison string
// is found contained in the output.
func Contains(compare string) test.Comparator {
	//nolint:thelper
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, strings.Contains(stdout, compare),
			fmt.Sprintf("Output does not contain: %q", compare)+info)
	}
}

// DoesNotContain is to be used for expected.Output to ensure a comparison string is NOT found in the output.
func DoesNotContain(compare string) test.Comparator {
	//nolint:thelper
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, !strings.Contains(stdout, compare),
			fmt.Sprintf("Output does contain: %q", compare)+info)
	}
}

// Equals is to be used for expected.Output to ensure it is exactly the output.
func Equals(compare string) test.Comparator {
	//nolint:thelper
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Equal(t, compare, stdout, info)
	}
}

// Match is to be used for expected.Output to ensure we match a regexp.
// Provisional - expected use, but have not seen it so far.
func Match(reg *regexp.Regexp) test.Comparator {
	//nolint:thelper
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Check(t, reg.MatchString(stdout), "Output does not match: "+reg.String(), info)
	}
}
