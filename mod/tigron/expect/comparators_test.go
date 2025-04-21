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
package expect_test

// TODO: add a lot more tests including failure conditions with mimicry

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

func TestExpect(t *testing.T) {
	// TODO: write more tests once we can mock t in Comparator signature
	t.Parallel()

	expect.Contains("b")("a b c", "contains works", t)
	expect.DoesNotContain("d")("a b c", "does not contain works", t)
	expect.Equals("a b c")("a b c", "equals work", t)
	expect.Match(regexp.MustCompile("[a-z ]+"))("a b c", "match works", t)

	expect.All(
		expect.Contains("b"),
		expect.Contains("b", "c"),
		expect.DoesNotContain("d"),
		expect.DoesNotContain("d", "e"),
		expect.Equals("a b c"),
		expect.Match(regexp.MustCompile("[a-z ]+")),
	)("a b c", "all", t)

	type foo struct {
		Foo map[string]string `json:"foo"`
	}

	data, err := json.Marshal(&foo{
		Foo: map[string]string{
			"foo": "bar",
		},
	})

	assertive.ErrorIsNil(t, err)

	expect.JSON(&foo{}, nil)(string(data), "json, no verifier", t)

	expect.JSON(&foo{}, func(obj *foo, info string, t tig.T) {
		assertive.IsEqual(t, obj.Foo["foo"], "bar", info)
	})(string(data), "json, with verifier", t)
}
