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

//revive:disable:add-constant
package assertive_test

import (
	"errors"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/internal/mimicry"
	"github.com/containerd/nerdctl/mod/tigron/internal/mocks"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

func TestAssertivePass(t *testing.T) {
	t.Parallel()

	var nilErr error
	//nolint:err113 // Fine, this is a test
	notNilErr := errors.New("some error")

	assertive.ErrorIsNil(t, nilErr, "a nil error should pass ErrorIsNil")
	assertive.ErrorIs(t, nilErr, nil, "a nil error should pass ErrorIs(err, nil)")
	assertive.ErrorIs(
		t,
		fmt.Errorf("neh %w", notNilErr),
		notNilErr,
		"an error wrapping another should match with ErrorIs",
	)

	assertive.IsEqual(t, "foo", "foo", "= should work as expected (on string)")
	assertive.IsNotEqual(t, "foo", "else", "!= should work as expected (on string)")

	assertive.IsEqual(t, true, true, "= should work as expected (on bool)")
	assertive.IsNotEqual(t, true, false, "!= should work as expected (on bool)")

	assertive.IsEqual(t, 1, 1, "= should work as expected (on int)")
	assertive.IsNotEqual(t, 1, 0, "!= should work as expected (on int)")

	assertive.IsEqual(t, -1.0, -1, "= should work as expected (on float)")
	assertive.IsNotEqual(t, -1.0, 0, "!= should work as expected (on float)")

	type foo struct {
		name string
	}

	assertive.IsEqual(t, foo{}, foo{}, "= should work as expected (on struct)")
	assertive.IsEqual(
		t,
		foo{name: "foo"},
		foo{name: "foo"},
		"= should work as expected (on struct)",
	)
	assertive.IsNotEqual(
		t,
		foo{name: "bar"},
		foo{name: "foo"},
		"!= should work as expected (on struct)",
	)

	assertive.Contains(t, "foo", "o", "⊂ should work")
	assertive.DoesNotContain(t, "foo", "a", "¬⊂ should work")
	assertive.HasPrefix(t, "foo", "f", "prefix should work")
	assertive.HasSuffix(t, "foo", "o", "suffix should work")
	assertive.Match(t, "foo", regexp.MustCompile("^[fo]{3,}$"), "match should work")
	assertive.DoesNotMatch(t, "foo", regexp.MustCompile("^[abc]{3,}$"), "match should work")

	assertive.True(t, true, "is true should work as expected")

	assertive.IsLessThan(t, time.Minute, time.Hour, "< should work (duration)")
	assertive.IsMoreThan(t, time.Minute, time.Second, "< should work (duration)")
	assertive.IsLessThan(t, 1, 2, "< should work (int)")
	assertive.IsMoreThan(t, 2, 1, "> should work (int)")
	assertive.IsLessThan(t, -1.2, 2, "< should work (float)")
	assertive.IsMoreThan(t, 2, -1.2, "> should work (float)")
}

func TestAssertiveFailBehavior(t *testing.T) {
	t.Parallel()

	mockT := &mocks.MockT{}

	var nilErr error
	//nolint:err113 // Fine, this is a test
	notNilErr := errors.New("some error")

	assertive.ErrorIsNil(mockT, notNilErr, "a nil error should pass ErrorIsNil")
	assertive.ErrorIs(mockT, notNilErr, nil, "a nil error should pass ErrorIs(err, nil)")
	assertive.ErrorIs(
		mockT,
		fmt.Errorf("neh %w", nilErr),
		nilErr,
		"an error wrapping another should match with ErrorIs",
	)

	assertive.IsEqual(mockT, "foo", "else", "= should work as expected (on string)")
	assertive.IsNotEqual(mockT, "foo", "foo", "!= should work as expected (on string)")

	assertive.IsEqual(mockT, true, false, "= should work as expected (on bool)")
	assertive.IsNotEqual(mockT, true, true, "!= should work as expected (on bool)")

	assertive.IsEqual(mockT, 1, 0, "= should work as expected (on int)")
	assertive.IsNotEqual(mockT, 1, 1, "!= should work as expected (on int)")

	assertive.IsEqual(mockT, -1.0, 0, "= should work as expected (on float)")
	assertive.IsNotEqual(mockT, -1.0, -1, "!= should work as expected (on float)")

	type foo struct {
		name string
	}

	assertive.IsEqual(mockT, foo{}, foo{name: "foo"}, "= should work as expected (on struct)")
	assertive.IsEqual(
		mockT,
		foo{name: "bar"},
		foo{name: "foo"},
		"= should work as expected (on struct)",
	)
	assertive.IsNotEqual(
		mockT,
		foo{name: ""},
		foo{name: ""},
		"!= should work as expected (on struct)",
	)

	assertive.Contains(mockT, "foo", "a", "⊂ should work")
	assertive.DoesNotContain(mockT, "foo", "o", "¬⊂ should work")
	assertive.HasPrefix(mockT, "foo", "o", "prefix should work")
	assertive.HasSuffix(mockT, "foo", "f", "suffix should work")
	assertive.Match(mockT, "foo", regexp.MustCompile("^[abc]{3,}$"), "match should work")
	assertive.DoesNotMatch(mockT, "foo", regexp.MustCompile("^[fo]{3,}$"), "match should work")

	assertive.True(mockT, false, "is true should work as expected")

	assertive.IsLessThan(mockT, time.Hour, time.Minute, "< should work (duration)")
	assertive.IsMoreThan(mockT, time.Second, time.Minute, "< should work (duration)")
	assertive.IsLessThan(mockT, 2, 1, "< should work (int)")
	assertive.IsMoreThan(mockT, 1, 2, "> should work (int)")
	assertive.IsLessThan(mockT, 2, -1.2, "< should work (float)")
	assertive.IsMoreThan(mockT, -1.2, 2, "> should work (float)")

	if len(mockT.Report(tig.T.FailNow)) != 27 {
		t.Error("we should have called FailNow as many times as we have asserts here")
	}

	if len(mockT.Report(tig.T.Fail)) != 0 {
		t.Error("we should NOT have called Fail")
	}
}

func TestAssertiveFailLater(t *testing.T) {
	t.Parallel()

	mockT := &mocks.MockT{}

	assertive.True(assertive.WithFailLater(mockT), false, "is true should work as expected")

	if len(mockT.Report(tig.T.FailNow)) != 0 {
		t.Log(mimicry.PrintCall(mockT.Report(tig.T.FailNow)[0]))
		t.Error("we should NOT have called FailNow")
	}

	if len(mockT.Report(tig.T.Fail)) != 1 {
		t.Error("we should have called Fail")
	}
}

func TestAssertiveSilentSuccess(t *testing.T) {
	t.Parallel()

	mockT := &mocks.MockT{}

	assertive.True(mockT, true, "is true should work as expected")
	assertive.True(mockT, false, "is true should work as expected")

	if len(mockT.Report(tig.T.Log)) != 2 {
		t.Error("we should have called Log on both success and failure")
	}

	mockT.Reset()

	assertive.True(assertive.WithSilentSuccess(mockT), true, "is true should work as expected")

	if len(mockT.Report(tig.T.Log)) != 0 {
		t.Log(mimicry.PrintCall(mockT.Report(tig.T.Log)[0]))
		t.Error("we should NOT have called Log on success")
	}

	assertive.True(assertive.WithSilentSuccess(mockT), false, "is true should work as expected")

	if len(mockT.Report(tig.T.Log)) != 1 {
		t.Error("we should still have called Log on failure")
	}
}
