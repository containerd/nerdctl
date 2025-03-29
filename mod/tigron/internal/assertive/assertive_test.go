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
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
)

func TestY(t *testing.T) {
	t.Parallel()

	var err error

	assertive.ErrorIsNil(t, err)

	//nolint:err113 // Fine, this is a test
	someErr := errors.New("test error")

	err = fmt.Errorf("wrap: %w", someErr)
	assertive.ErrorIs(t, err, someErr)

	foo := "foo"
	assertive.IsEqual(t, foo, "foo")

	bar := 10
	assertive.IsEqual(t, bar, 10)

	baz := true
	assertive.IsEqual(t, baz, true)
}
