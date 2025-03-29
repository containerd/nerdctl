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

//revive:disable:add-constant,package-comments
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
	Log(args ...any)
}

// ErrorIsNil immediately fails a test if err is not nil.
func ErrorIsNil(testing testingT, err error, msg ...string) {
	testing.Helper()

	if err != nil {
		testing.Log("expecting nil error, but got:", err)
		failNow(testing, msg...)
	}
}

// ErrorIs immediately fails a test if err is not the comparison error.
func ErrorIs(testing testingT, err, compErr error, msg ...string) {
	testing.Helper()

	if !errors.Is(err, compErr) {
		testing.Log("expected error to be:", compErr, "- instead it is:", err)
		failNow(testing, msg...)
	}
}

// IsEqual immediately fails a test if the two interfaces are not equal.
func IsEqual(testing testingT, actual, expected any, msg ...string) {
	testing.Helper()

	if !equal(testing, actual, expected) {
		testing.Log("expected:", actual, " - to be equal to:", expected)
		failNow(testing, msg...)
	}
}

// IsNotEqual immediately fails a test if the two interfaces are equal.
func IsNotEqual(testing testingT, actual, expected any, msg ...string) {
	testing.Helper()

	if equal(testing, actual, expected) {
		testing.Log("expected:", actual, " - to be equal to:", expected)
		failNow(testing, msg...)
	}
}

// StringContains immediately fails a test if the actual string does not contain the other string.
func StringContains(testing testingT, actual, contains string, msg ...string) {
	testing.Helper()

	if !strings.Contains(actual, contains) {
		testing.Log("expected:", actual, " - to contain:", contains)
		failNow(testing, msg...)
	}
}

// StringDoesNotContain immediately fails a test if the actual string contains the other string.
func StringDoesNotContain(testing testingT, actual, contains string, msg ...string) {
	testing.Helper()

	if strings.Contains(actual, contains) {
		testing.Log("expected:", actual, " - to NOT contain:", contains)
		failNow(testing, msg...)
	}
}

// StringHasSuffix immediately fails a test if the string does not end with suffix.
func StringHasSuffix(testing testingT, actual, suffix string, msg ...string) {
	testing.Helper()

	if !strings.HasSuffix(actual, suffix) {
		testing.Log("expected:", actual, " - to end with:", suffix)
		failNow(testing, msg...)
	}
}

// StringHasPrefix immediately fails a test if the string does not start with prefix.
func StringHasPrefix(testing testingT, actual, prefix string, msg ...string) {
	testing.Helper()

	if !strings.HasPrefix(actual, prefix) {
		testing.Log("expected:", actual, " - to start with:", prefix)
		failNow(testing, msg...)
	}
}

// DurationIsLessThan immediately fails a test if the duration is more than the reference.
func DurationIsLessThan(testing testingT, actual, expected time.Duration, msg ...string) {
	testing.Helper()

	if actual >= expected {
		testing.Log("expected:", actual, " - to be less than:", expected)
		failNow(testing, msg...)
	}
}

// True immediately fails a test if the boolean is not true...
func True(testing testingT, comp bool, msg ...string) bool {
	testing.Helper()

	if !comp {
		failNow(testing, msg...)
	}

	return comp
}

// Check marks a test as failed if the boolean is not true (safe in go routines).
func Check(testing testingT, comp bool, msg ...string) bool {
	testing.Helper()

	if !comp {
		for _, m := range msg {
			testing.Log(m)
		}

		testing.Fail()
	}

	return comp
}

func failNow(testing testingT, msg ...string) {
	testing.Helper()

	if len(msg) > 0 {
		for _, m := range msg {
			testing.Log(m)
		}
	}

	testing.FailNow()
}

func equal(testing testingT, actual, expected any) bool {
	testing.Helper()

	// FIXME: this is risky and limited. Right now this is fine internally, but do better if this
	// becomes public.
	return actual == expected
}
