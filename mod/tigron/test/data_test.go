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
package test

import (
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
)

func TestDataBasic(t *testing.T) {
	t.Parallel()

	dataObj := WithData("test", "create")

	assertive.IsEqual(t, dataObj.Get("test"), "create")
	assertive.IsEqual(t, dataObj.Get("doesnotexist"), "")

	dataObj.Set("test", "set")
	assertive.IsEqual(t, dataObj.Get("test"), "set")
}

func TestDataTempDir(t *testing.T) {
	t.Parallel()

	dataObj := configureData(t, nil, nil)

	one := dataObj.TempDir()
	two := dataObj.TempDir()

	assertive.IsEqual(t, one, two)
	assertive.IsNotEqual(t, one, "")
}

func TestDataIdentifier(t *testing.T) {
	t.Parallel()

	dataObj := configureData(t, nil, nil)

	one := dataObj.Identifier()
	two := dataObj.Identifier()

	assertive.IsEqual(t, one, two)
	assertive.StringHasPrefix(t, one, "testdataidentifier")

	three := dataObj.Identifier("Some Add ∞ Funky∞Prefix")
	assertive.StringHasPrefix(t, three, "testdataidentifier-some-add-funky-prefix")
}

func TestDataIdentifierThatIsReallyReallyReallyReallyReallyReallyReallyReallyReallyReallyReallyLong(
	t *testing.T,
) {
	t.Parallel()

	dataObj := configureData(t, nil, nil)

	one := dataObj.Identifier()
	two := dataObj.Identifier()

	assertive.IsEqual(t, one, two)
	assertive.StringHasPrefix(t, one, "testdataidentifier")
	assertive.IsEqual(t, len(one), identifierMaxLength)

	three := dataObj.Identifier("Add something")
	assertive.IsNotEqual(t, three, one)
}
