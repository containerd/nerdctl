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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Masterminds/semver/v3"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
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

func RequireKernelVersion(t testing.TB, constraint string) {
	t.Helper()
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		t.Fatal(err)
	}
	// EL kernel versions are not semver, so, cleanup first
	un := strings.Split(infoutil.UnameR(), "-")[0]
	unameR, err := semver.NewVersion(un)
	if err != nil {
		t.Skip(err)
	}
	if !c.Check(unameR) {
		t.Skipf("version %v does not satisfy constraints %v", unameR, c)
	}
}

func RequireSystemService(t testing.TB, sv string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skipf("Service %q is not supported on %q", sv, runtime.GOOS)
	}
	var systemctlArgs []string
	if rootlessutil.IsRootless() {
		systemctlArgs = append(systemctlArgs, "--user")
	}
	systemctlArgs = append(systemctlArgs, []string{"-q", "is-active", sv}...)
	cmd := exec.Command("systemctl", systemctlArgs...)
	if err := cmd.Run(); err != nil {
		t.Skipf("Service %q does not seem active: %v: %v", sv, cmd.Args, err)
	}
}

const Namespace = "nerdctl-test"
