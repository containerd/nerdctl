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

//nolint:testpackage
//revive:disable:add-constant
package test

import (
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
)

func TestLabels(t *testing.T) {
	t.Parallel()

	dataLabels := WithLabels(map[string]string{"test": "create"}).Labels()

	assertive.IsEqual(t, dataLabels.Get("test"), "create")
	assertive.IsEqual(t, dataLabels.Get("doesnotexist"), "")

	dataLabels.Set("test", "set")
	assertive.IsEqual(t, dataLabels.Get("test"), "set")

	dataLabels.Set("test", "reset")
	assertive.IsEqual(t, dataLabels.Get("test"), "reset")
}

func TestTemp(t *testing.T) {
	t.Parallel()

	dataObj := newData(t, nil, nil)

	one := dataObj.Temp().Path()
	two := dataObj.Temp().Path()

	assertive.IsEqual(t, one, two)
	assertive.IsNotEqual(t, one, "")

	t.Run("verify that subtest has an independent TempDir", func(t *testing.T) {
		t.Parallel()

		dataObj = newData(t, nil, nil)
		three := dataObj.Temp().Path()
		assertive.IsNotEqual(t, one, three)
	})
}

func TestDataIdentifier(t *testing.T) {
	t.Parallel()

	dataObj := newData(t, nil, nil)

	one := dataObj.Identifier()
	two := dataObj.Identifier()

	assertive.IsEqual(t, one, two)
	assertive.HasPrefix(t, one, "testdataidentifier")

	three := dataObj.Identifier("Some Add ∞ Funky∞Prefix")
	assertive.HasPrefix(t, three, "testdataidentifier-some-add-funky-prefix")
}

func TestDataIdentifierThatIsReallyReallyReallyReallyReallyReallyReallyReallyReallyReallyReallyLong(
	t *testing.T,
) {
	t.Parallel()

	dataObj := newData(t, nil, nil)

	one := dataObj.Identifier()
	two := dataObj.Identifier()

	assertive.IsEqual(t, one, two)
	assertive.HasPrefix(t, one, "testdataidentifier")
	assertive.IsEqual(t, len(one), identifierMaxLength)

	three := dataObj.Identifier("Add something")
	assertive.IsNotEqual(t, three, one)
}
