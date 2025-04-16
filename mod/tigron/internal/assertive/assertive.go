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
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/formatter"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

const (
	markLineLength           = 20
	expectedSuccessDecorator = "✅️ does verify:\t\t"
	expectedFailDecorator    = "❌ FAILED!\t\t"
	receivedDecorator        = "👀 testing:\t\t"
	annotationDecorator      = "🖊️"
	hyperlinkDecorator       = "🔗"
)

// ErrorIsNil fails a test if err is not nil.
func ErrorIsNil(testing tig.T, err error, msg ...string) {
	testing.Helper()

	evaluate(testing, errors.Is(err, nil), err, "is `<nil>`", msg...)
}

// ErrorIs fails a test if err is not the comparison error.
func ErrorIs(testing tig.T, err, expected error, msg ...string) {
	testing.Helper()

	evaluate(testing, errors.Is(err, expected), err, fmt.Sprintf("is `%v`", expected), msg...)
}

// IsEqual fails a test if the two interfaces are not equal.
func IsEqual[T comparable](testing tig.T, actual, expected T, msg ...string) {
	testing.Helper()

	evaluate(testing, actual == expected, actual, fmt.Sprintf("= `%v`", expected), msg...)
}

// IsNotEqual fails a test if the two interfaces are equal.
func IsNotEqual[T comparable](testing tig.T, actual, expected T, msg ...string) {
	testing.Helper()

	evaluate(testing, actual != expected, actual, fmt.Sprintf("!= `%v`", expected), msg...)
}

// Contains fails a test if the actual string does not contain the other string.
func Contains(testing tig.T, actual, contains string, msg ...string) {
	testing.Helper()

	evaluate(
		testing,
		strings.Contains(actual, contains),
		actual,
		fmt.Sprintf("~= `%v`", contains),
		msg...)
}

// DoesNotContain fails a test if the actual string contains the other string.
func DoesNotContain(testing tig.T, actual, contains string, msg ...string) {
	testing.Helper()

	evaluate(
		testing,
		!strings.Contains(actual, contains),
		actual,
		fmt.Sprintf("! ~= `%v`", contains),
		msg...)
}

// HasSuffix fails a test if the string does not end with suffix.
func HasSuffix(testing tig.T, actual, suffix string, msg ...string) {
	testing.Helper()

	evaluate(
		testing,
		strings.HasSuffix(actual, suffix),
		actual,
		fmt.Sprintf("`%v` $", suffix),
		msg...)
}

// HasPrefix fails a test if the string does not start with prefix.
func HasPrefix(testing tig.T, actual, prefix string, msg ...string) {
	testing.Helper()

	evaluate(
		testing,
		strings.HasPrefix(actual, prefix),
		actual,
		fmt.Sprintf("^ `%v`", prefix),
		msg...)
}

// Match fails a test if the string does not match the regexp.
func Match(testing tig.T, actual string, reg *regexp.Regexp, msg ...string) {
	testing.Helper()

	evaluate(testing, reg.MatchString(actual), actual, fmt.Sprintf("`%v`", reg), msg...)
}

// DoesNotMatch fails a test if the string does match the regexp.
func DoesNotMatch(testing tig.T, actual string, reg *regexp.Regexp, msg ...string) {
	testing.Helper()

	evaluate(testing, !reg.MatchString(actual), actual, fmt.Sprintf("`%v`", reg), msg...)
}

// IsLessThan fails a test if the actual is more or equal than the reference.
func IsLessThan[T ~int | ~float64 | time.Duration](
	testing tig.T,
	actual, expected T,
	msg ...string,
) {
	testing.Helper()

	evaluate(testing, actual < expected, actual, fmt.Sprintf("< `%v`", expected), msg...)
}

// IsMoreThan fails a test if the actual is less or equal than the reference.
func IsMoreThan[T ~int | ~float64 | time.Duration](
	testing tig.T,
	actual, expected T,
	msg ...string,
) {
	testing.Helper()

	evaluate(testing, actual > expected, actual, fmt.Sprintf("< `%v`", expected), msg...)
}

// True fails a test if the boolean is not true...
func True(testing tig.T, comp bool, msg ...string) bool {
	testing.Helper()

	evaluate(testing, comp, comp, true, msg...)

	return comp
}

// WithFailLater will allow an assertion to not fail the test immediately.
// Failing later is necessary when asserting inside go routines, and also if you want many
// successive asserts to all evaluate instead of stopping at the first failing one.
// FIXME: it should be possible to have both WithFailLater and WithSilentSuccess at the same time.
func WithFailLater(t tig.T) tig.T {
	return &failLater{
		t,
	}
}

// WithSilentSuccess (used to wrap a *testing.T struct) will not log debugging assertive information
// when the result is
// a success.
// In some cases, this is convenient to avoid crowding the display with successful checks info.
func WithSilentSuccess(t tig.T) tig.T {
	return &silentSuccess{
		t,
	}
}

type failLater struct {
	tig.T
}
type silentSuccess struct {
	tig.T
}

func evaluate(testing tig.T, isSuccess bool, actual, expected any, msg ...string) {
	testing.Helper()

	decorate(testing, isSuccess, actual, expected, msg...)

	if !isSuccess {
		if _, ok := testing.(*failLater); ok {
			testing.Fail()
		} else {
			testing.FailNow()
		}
	}
}

func decorate(testing tig.T, isSuccess bool, actual, expected any, msg ...string) {
	testing.Helper()

	if _, ok := testing.(*silentSuccess); !isSuccess || !ok {
		head := strings.Repeat("<", markLineLength)
		footer := strings.Repeat(">", markLineLength)
		header := "\t"

		custom := fmt.Sprintf("\t%s %s", annotationDecorator, strings.Join(msg, "\n"))

		msg = append([]string{"", head}, custom)

		msg = append([]string{getTopFrameFile()}, msg...)

		msg = append(msg, fmt.Sprintf("\t%s`%v`", receivedDecorator, actual))

		if isSuccess {
			msg = append(msg,
				fmt.Sprintf("\t%s%v", expectedSuccessDecorator, expected),
			)
		} else {
			msg = append(msg,
				fmt.Sprintf("\t%s%v", expectedFailDecorator, expected),
			)
		}

		testing.Log(header + strings.Join(msg, "\n") + "\n" + footer + "\n")
	}
}

// XXX FIXME #expert
// Because of how golang testing works, the upper frame is the one from where t.Run is being called,
// as (presumably) the passed function is starting with its own stack in a go routine.
// In the case of subtests, t.Run being called from inside Tigron will make it so that the top frame
// is case.go around line 233 (where we call Command.Run(), which is the one calling assertive).
// To possibly address this:
// plan a. just drop entirely OSC8 links and source extracts and trash all of this
// plan b. get the top frame from the root test, and pass it to subtests on a custom property, the somehow into here
// plan c. figure out a hack to call t.Run from the test file without ruining the Tigron UX
// Dereference t.Run? Return a closure to be called from the top? w/o disabling inlining in the right place?
// Short term, blacklisting /tigron (and /nerdtest) will at least prevent the wrong links from appearing in the output.
func getTopFrameFile() string {
	// Get the frames. Skip the first two frames - current one and caller.
	//nolint:mnd // Whatever mnd...
	pc := make([]uintptr, 40)
	//nolint:mnd // Whatever mnd...
	n := runtime.Callers(2, pc)
	callersFrames := runtime.CallersFrames(pc[:n])

	var (
		file       string
		lineNumber int
		frame      runtime.Frame
	)

	more := true
	for more {
		frame, more = callersFrames.Next()

		// Once we are in the go main stack, bail out
		if !strings.Contains(frame.Function, "/") {
			break
		}

		// XXX see note above
		if strings.Contains(frame.File, "/tigron") {
			continue
		}

		if strings.Contains(frame.File, "/nerdtest") {
			continue
		}

		file = frame.File
		lineNumber = frame.Line
	}

	if file == "" {
		return ""
	}

	//nolint:gosec // file is coming from runtime frames so, fine
	source, err := os.Open(file)
	if err != nil {
		return ""
	}

	defer func() {
		_ = source.Close()
	}()

	index := 1
	scanner := bufio.NewScanner(source)

	var line string

	for ; scanner.Err() == nil && index <= lineNumber; index++ {
		if !scanner.Scan() {
			break
		}

		line = strings.Trim(scanner.Text(), "\t ")
	}

	return hyperlinkDecorator + " " + (&formatter.OSC8{
		Text:     line,
		Location: "file://" + file,
		Line:     lineNumber,
	}).String()
}
