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

package require_test

import (
	"runtime"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/require"
)

const (
	darwin  = "darwin"
	windows = "windows"
	linux   = "linux"
)

func TestRequire(t *testing.T) {
	t.Parallel()

	var pass bool

	switch runtime.GOOS {
	case "windows":
		pass, _ = require.Windows.Check(nil, nil)
	case "linux":
		pass, _ = require.Linux.Check(nil, nil)
	case "darwin":
		pass, _ = require.Darwin.Check(nil, nil)
	default:
		pass, _ = require.OS(runtime.GOOS).Check(nil, nil)
	}

	assertive.IsEqual(t, pass, true)

	switch runtime.GOARCH {
	case "amd64":
		pass, _ = require.Amd64.Check(nil, nil)
	case "arm64":
		pass, _ = require.Arm64.Check(nil, nil)
	default:
		pass, _ = require.Arch(runtime.GOARCH).Check(nil, nil)
	}

	assertive.IsEqual(t, pass, true, "Require works as expected")
}

func TestNot(t *testing.T) {
	t.Parallel()

	var pass bool

	switch runtime.GOOS {
	case windows:
		pass, _ = require.Not(require.Linux).Check(nil, nil)
	case linux:
		pass, _ = require.Not(require.Windows).Check(nil, nil)
	case darwin:
		pass, _ = require.Not(require.Windows).Check(nil, nil)
	default:
		pass, _ = require.Not(require.Linux).Check(nil, nil)
	}

	assertive.IsEqual(t, pass, true, "require.Not works as expected")
}

func TestAllSuccess(t *testing.T) {
	t.Parallel()

	var pass bool

	switch runtime.GOOS {
	case windows:
		pass, _ = require.All(require.Not(require.Linux), require.Not(require.Darwin)).
			Check(nil, nil)
	case linux:
		pass, _ = require.All(require.Not(require.Windows), require.Not(require.Darwin)).
			Check(nil, nil)
	case darwin:
		pass, _ = require.All(require.Not(require.Windows), require.Not(require.Linux)).
			Check(nil, nil)
	default:
		pass, _ = require.All(require.Not(require.Windows), require.Not(require.Linux),
			require.Not(require.Darwin)).Check(nil, nil)
	}

	assertive.IsEqual(t, pass, true, "require.All works as expected")
}

func TestAllOneFail(t *testing.T) {
	t.Parallel()

	var pass bool

	switch runtime.GOOS {
	case "windows":
		pass, _ = require.All(require.Not(require.Linux), require.Not(require.Darwin)).
			Check(nil, nil)
	case "linux":
		pass, _ = require.All(require.Not(require.Windows), require.Not(require.Darwin)).
			Check(nil, nil)
	case "darwin":
		pass, _ = require.All(require.Not(require.Windows), require.Not(require.Linux)).
			Check(nil, nil)
	default:
		pass, _ = require.All(require.Not(require.OS(runtime.GOOS)), require.Not(require.Linux),
			require.Not(require.Darwin)).Check(nil, nil)
	}

	assertive.IsEqual(t, pass, true, "mixing require.All and require.Not works as expected")
}
