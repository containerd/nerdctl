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

package reflectutil

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnknownNonEmptyFields(t *testing.T) {
	type foo struct {
		FooBool    bool
		FooBoolPtr *bool
		FooMap     map[string]struct{}
		FooStr     string
		FooStr2    string
		FooStr3    string
	}

	foo1 := foo{
		FooBool:    true,
		FooBoolPtr: nil,
		FooMap:     nil,
		FooStr:     "foo",
		FooStr2:    "",
		FooStr3:    "oof",
	}
	assert.DeepEqual(t,
		[]string{"FooStr", "FooStr3"},
		UnknownNonEmptyFields(&foo1, "FooBool"))
	assert.DeepEqual(t,
		[]string{"FooStr", "FooStr3"},
		UnknownNonEmptyFields(foo1, "FooBool"))
	assert.DeepEqual(t,
		[]string{"FooStr", "FooStr3"},
		UnknownNonEmptyFields(&foo1, "FooBool", "FooMap"))

	foo2 := foo1
	foo2.FooBoolPtr = &foo1.FooBool
	foo2.FooMap = map[string]struct{}{
		"blah": {},
	}
	assert.DeepEqual(t,
		[]string{"FooBoolPtr", "FooMap", "FooStr", "FooStr3"},
		UnknownNonEmptyFields(&foo2, "FooBool"))

	foo3 := foo1
	foo3.FooMap = make(map[string]struct{})
	assert.DeepEqual(t,
		[]string{"FooStr", "FooStr3"},
		UnknownNonEmptyFields(&foo3, "FooBool"))
}
