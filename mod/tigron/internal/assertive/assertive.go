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

package assertive

import (
	"errors"
	"strings"
	"time"
)

type testingT interface {
	Helper()
	FailNow()
	Fail()
	Log(args ...interface{})
}

// ErrorIsNil immediately fails a test if err is not nil.
func ErrorIsNil(t testingT, err error, msg ...string) {
	t.Helper()

	if err != nil {
		t.Log("expecting nil error, but got:", err)
		failNow(t, msg...)
	}
}

// ErrorIs immediately fails a test if err is not the comparison error.
func ErrorIs(t testingT, err, compErr error, msg ...string) {
	t.Helper()

	if !errors.Is(err, compErr) {
		t.Log("expected error to be:", compErr, "- instead it is:", err)
		failNow(t, msg...)
	}
}

// IsEqual immediately fails a test if the two interfaces are not equal.
func IsEqual(t testingT, actual, expected interface{}, msg ...string) {
	t.Helper()

	if !isEqual(t, actual, expected) {
		t.Log("expected:", actual, " - to be equal to:", expected)
		failNow(t, msg...)
	}
}

// IsNotEqual immediately fails a test if the two interfaces are equal.
func IsNotEqual(t testingT, actual, expected interface{}, msg ...string) {
	t.Helper()

	if isEqual(t, actual, expected) {
		t.Log("expected:", actual, " - to be equal to:", expected)
		failNow(t, msg...)
	}
}

// StringContains immediately fails a test if the actual string does not contain the other string.
func StringContains(t testingT, actual, contains string, msg ...string) {
	t.Helper()

	if !strings.Contains(actual, contains) {
		t.Log("expected:", actual, " - to contain:", contains)
		failNow(t, msg...)
	}
}

// StringDoesNotContain immediately fails a test if the actual string contains the other string.
func StringDoesNotContain(t testingT, actual, contains string, msg ...string) {
	t.Helper()

	if strings.Contains(actual, contains) {
		t.Log("expected:", actual, " - to NOT contain:", contains)
		failNow(t, msg...)
	}
}

// StringHasSuffix immediately fails a test if the string does not end with suffix.
func StringHasSuffix(t testingT, actual, suffix string, msg ...string) {
	t.Helper()

	if !strings.HasSuffix(actual, suffix) {
		t.Log("expected:", actual, " - to end with:", suffix)
		failNow(t, msg...)
	}
}

// StringHasPrefix immediately fails a test if the string does not start with prefix.
func StringHasPrefix(t testingT, actual, prefix string, msg ...string) {
	t.Helper()

	if !strings.HasPrefix(actual, prefix) {
		t.Log("expected:", actual, " - to start with:", prefix)
		failNow(t, msg...)
	}
}

// DurationIsLessThan immediately fails a test if the duration is more than the reference.
func DurationIsLessThan(t testingT, actual, expected time.Duration, msg ...string) {
	t.Helper()

	if actual >= expected {
		t.Log("expected:", actual, " - to be less than:", expected)
		failNow(t, msg...)
	}
}

// True immediately fails a test if the boolean is not true...
func True(t testingT, comp bool, msg ...string) bool {
	t.Helper()

	if !comp {
		failNow(t, msg...)
	}

	return comp
}

// Check marks a test as failed if the boolean is not true (safe in go routines)
//
//nolint:varnamelen
func Check(t testingT, comp bool, msg ...string) bool {
	t.Helper()

	if !comp {
		for _, m := range msg {
			t.Log(m)
		}

		t.Fail()
	}

	return comp
}

//nolint:varnamelen
func failNow(t testingT, msg ...string) {
	t.Helper()

	if len(msg) > 0 {
		for _, m := range msg {
			t.Log(m)
		}
	}

	t.FailNow()
}

func isEqual(t testingT, actual, expected interface{}) bool {
	t.Helper()

	// FIXME: this is risky and limited. Right now this is fine internally, but do better if this
	// becomes public.
	return actual == expected
}
