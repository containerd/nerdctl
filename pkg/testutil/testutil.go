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

package testutil

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
)

var (
	flagTestTarget      string
	flagTestKillDaemon  bool
	flagTestIPv6        bool
	flagTestKube        bool
	flagTestFlaky       bool
	flagTestModifyUsers bool
)

var (
	testLockFile = filepath.Join(os.TempDir(), "nerdctl-test-prevent-concurrency", ".lock")
)

func M(m *testing.M) {
	flag.StringVar(&flagTestTarget, "test.target", "nerdctl", "target to test")
	flag.BoolVar(&flagTestKillDaemon, "test.allow-kill-daemon", false, "enable tests that kill the daemon")
	flag.BoolVar(&flagTestModifyUsers, "test.allow-modify-users", false, "enable tests that creates/deletes user accounts on the host")
	flag.BoolVar(&flagTestIPv6, "test.only-ipv6", false, "enable tests on IPv6")
	flag.BoolVar(&flagTestKube, "test.only-kubernetes", false, "enable tests on Kubernetes")
	flag.BoolVar(&flagTestFlaky, "test.only-flaky", false, "enable testing of flaky tests only (if false, flaky tests are ignored)")
	flag.Parse()

	if flagTestTarget == "" {
		flagTestTarget = "nerdctl"
	}

	os.Exit(func() int {
		err := os.MkdirAll(filepath.Dir(testLockFile), 0o777)
		if err != nil {
			log.L.WithError(err).Errorf("failed creating testing lock directory %q", filepath.Dir(testLockFile))
			return 1
		}

		// Ensure that permissions are set to 777 (regardless of umask value), so that we do not lock people out when
		// switching between rootful and rootless locking
		os.Chmod(filepath.Dir(testLockFile), 0o777)

		// Acquire lock
		lock, err := filesystem.Lock(filepath.Dir(testLockFile))
		if err != nil {
			log.L.WithError(err).Errorf("failed acquiring testing lock %q", filepath.Dir(testLockFile))
			return 1
		}

		// Release...
		defer filesystem.Unlock(lock)

		// Create marker file
		err = filesystem.WriteFile(testLockFile, []byte("prevent testing from running in parallel for subpackages integration tests"), 0o666)
		if err != nil {
			log.L.WithError(err).Errorf("failed writing lock file %q", testLockFile)
			return 1
		}

		// Ensure cleanup
		defer func() {
			os.Remove(testLockFile)
		}()

		// Now, run the tests
		fmt.Fprintf(os.Stderr, "test target: %q\n", flagTestTarget)

		return m.Run()
	}())
}

func GetTarget() string {
	if flagTestTarget == "" {
		panic("GetTarget() was called without calling M()")
	}
	return flagTestTarget
}

func GetEnableIPv6() bool {
	return flagTestIPv6
}

func GetEnableKubernetes() bool {
	return flagTestKube
}

func GetFlakyEnvironment() bool {
	return flagTestFlaky
}

func GetDaemonIsKillable() bool {
	return flagTestKillDaemon
}

func GetAllowModifyUsers() bool {
	return flagTestModifyUsers
}

const Namespace = "nerdctl-test"
