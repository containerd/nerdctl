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

import (
	"regexp"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/expect"
)

func TestExpect(t *testing.T) {
	t.Parallel()

	expect.Contains("b")("a b c", "info", t)
	expect.DoesNotContain("d")("a b c", "info", t)
	expect.Equals("a b c")("a b c", "info", t)
	expect.Match(regexp.MustCompile("[a-z ]+"))("a b c", "info", t)

	expect.All(
		expect.Contains("b"),
		expect.DoesNotContain("d"),
		expect.Equals("a b c"),
		expect.Match(regexp.MustCompile("[a-z ]+")),
	)("a b c", "info", t)
}
