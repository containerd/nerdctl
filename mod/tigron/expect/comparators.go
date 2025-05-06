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

//revive:disable:package-comments // annoying false positive behavior
//nolint:thelper // FIXME: remove when we move to tig.T
package expect

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

// All can be used as a parameter for expected.Output to group a set of comparators.
func All(comparators ...test.Comparator) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()

		for _, comparator := range comparators {
			comparator(stdout, "", t)
		}
	}
}

// Contains can be used as a parameter for expected.Output and ensures a comparison string is found contained in the
// output.
func Contains(compare string, more ...string) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()

		assertive.Contains(assertive.WithFailLater(t), stdout, compare, "Inspecting output (contains)")

		for _, m := range more {
			assertive.Contains(assertive.WithFailLater(t), stdout, m, "Inspecting output (contains)")
		}
	}
}

// DoesNotContain is to be used for expected.Output to ensure a comparison string is NOT found in the output.
func DoesNotContain(compare string, more ...string) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()

		assertive.DoesNotContain(assertive.WithFailLater(t), stdout, compare, "Inspecting output (does not contain)")

		for _, m := range more {
			assertive.DoesNotContain(assertive.WithFailLater(t), stdout, m, "Inspecting output (does not contain)")
		}
	}
}

// Equals is to be used for expected.Output to ensure it is exactly the output.
func Equals(compare string) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()
		assertive.IsEqual(assertive.WithFailLater(t), stdout, compare, "Inspecting output (equals)")
	}
}

// Match is to be used for expected.Output to ensure we match a regexp.
func Match(reg *regexp.Regexp) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()
		assertive.Match(assertive.WithFailLater(t), stdout, reg, "Inspecting output (match)")
	}
}

// JSON allows to verify that the output can be marshalled into T, and optionally can be further verified by a provided
// method.
func JSON[T any](obj T, verifier func(T, string, tig.T)) test.Comparator {
	return func(stdout, _ string, t *testing.T) {
		t.Helper()

		err := json.Unmarshal([]byte(stdout), &obj)
		assertive.ErrorIsNil(assertive.WithSilentSuccess(t), err, "Unmarshalling JSON from stdout must succeed")

		if verifier != nil && err == nil {
			verifier(obj, "Inspecting output (JSON)", t)
		}
	}
}
