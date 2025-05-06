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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"

	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/buildkitutil"
	"github.com/containerd/nerdctl/v2/pkg/imgutil"
	"github.com/containerd/nerdctl/v2/pkg/infoutil"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/v2/pkg/lockutil"
	"github.com/containerd/nerdctl/v2/pkg/platformutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
)

type Base struct {
	T                    testing.TB
	Target               string
	DaemonIsKillable     bool
	EnableIPv6           bool
	IPv6Compatible       bool
	EnableKubernetes     bool
	KubernetesCompatible bool
	Binary               string
	Args                 []string
	Env                  []string
	Dir                  string
}

// WithStdin sets the standard input of Cmd to the specified reader
func WithStdin(r io.Reader) func(*Cmd) {
	return func(i *Cmd) {
		i.Cmd.Stdin = r
	}
}

func (b *Base) Cmd(args ...string) *Cmd {
	icmdCmd := icmd.Command(b.Binary, append(b.Args, args...)...)
	icmdCmd.Env = b.Env
	icmdCmd.Dir = b.Dir
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

// ComposeCmd executes `nerdctl -n nerdctl-test compose` or `docker-compose`
func (b *Base) ComposeCmd(args ...string) *Cmd {
	binary := b.Binary
	binaryArgs := append(b.Args, append([]string{"compose"}, args...)...)
	icmdCmd := icmd.Command(binary, binaryArgs...)
	icmdCmd.Env = b.Env
	icmdCmd.Dir = b.Dir
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

func (b *Base) ComposeCmdWithHelper(helper []string, args ...string) *Cmd {
	helperBin, err := exec.LookPath(helper[0])
	if err != nil {
		b.T.Skipf("helper binary %q not found", helper[0])
	}
	binary := b.Binary
	binaryArgs := append(b.Args, append([]string{"compose"}, args...)...)

	helperArgs := helper[1:]
	helperArgs = append(helperArgs, binary)
	helperArgs = append(helperArgs, binaryArgs...)
	icmdCmd := icmd.Command(helperBin, helperArgs...)
	icmdCmd.Env = b.Env
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

func (b *Base) CmdWithHelper(helper []string, args ...string) *Cmd {
	helperBin, err := exec.LookPath(helper[0])
	if err != nil {
		b.T.Skipf("helper binary %q not found", helper[0])
	}
	helperArgs := helper[1:]
	helperArgs = append(helperArgs, b.Binary)
	helperArgs = append(helperArgs, b.Args...)
	helperArgs = append(helperArgs, args...)

	icmdCmd := icmd.Command(helperBin, helperArgs...)
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

func (b *Base) systemctlTarget() string {
	if IsDocker() {
		return "docker.service"
	}

	return "containerd.service"
}

func (b *Base) systemctlArgs() []string {
	var systemctlArgs []string
	if os.Geteuid() != 0 {
		systemctlArgs = append(systemctlArgs, "--user")
	}
	return systemctlArgs
}

func (b *Base) KillDaemon() {
	b.T.Helper()
	if !b.DaemonIsKillable {
		b.T.Skip("daemon is not killable (hint: set \"-test.allow-kill-daemon\")")
	}
	target := b.systemctlTarget()
	b.T.Logf("killing %q", target)
	cmdKill := exec.Command("systemctl",
		append(b.systemctlArgs(),
			[]string{"kill", target}...)...)
	if out, err := cmdKill.CombinedOutput(); err != nil {
		err = fmt.Errorf("cannot kill %q: %q: %w", target, string(out), err)
		b.T.Fatal(err)
	}
	// the daemon should restart automatically
}

func (b *Base) EnsureDaemonActive() {
	b.T.Helper()
	target := b.systemctlTarget()
	b.T.Logf("checking activity of %q", target)
	systemctlArgs := b.systemctlArgs()
	const (
		maxRetry = 30
		sleep    = 3 * time.Second
	)
	for i := 0; i < maxRetry; i++ {
		cmd := exec.Command("systemctl", append(systemctlArgs, "is-active", target)...)
		out, err := cmd.CombinedOutput()
		b.T.Logf("(retry=%d) %s", i, string(out))
		if err == nil {
			// The daemon is now running, but the daemon may still refuse connections to containerd.sock
			b.T.Logf("daemon %q is now running, checking whether the daemon can handle requests", target)
			infoRes := b.Cmd("info").Run()
			if infoRes.ExitCode == 0 {
				b.T.Logf("daemon %q can now handle requests", target)
				return
			}
			b.T.Logf("(retry=%d) %s", i, infoRes.Combined())
		}
		time.Sleep(sleep)
	}
	b.T.Fatalf("daemon %q not running?", target)
}

func (b *Base) DumpDaemonLogs(minutes int) {
	b.T.Helper()
	target := b.systemctlTarget()
	cmd := exec.Command("journalctl",
		append(b.systemctlArgs(), "-u", target, "--no-pager", "-S", fmt.Sprintf("%d min ago", minutes))...)
	b.T.Logf("===== %v =====", cmd.Args)
	out, err := cmd.CombinedOutput()
	if err != nil {
		b.T.Fatal(err)
	}
	b.T.Log(string(out))
	b.T.Log("==========")
}

func (b *Base) InspectContainer(name string) dockercompat.Container {
	cmdResult := b.Cmd("container", "inspect", name).Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var dc []dockercompat.Container
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &dc); err != nil {
		b.T.Fatal(err)
	}
	assert.Equal(b.T, 1, len(dc))
	return dc[0]
}

func (b *Base) InspectImage(name string) dockercompat.Image {
	cmdResult := b.Cmd("image", "inspect", name).Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var dc []dockercompat.Image
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &dc); err != nil {
		b.T.Fatal(err)
	}
	assert.Equal(b.T, 1, len(dc))
	return dc[0]
}

func (b *Base) InspectNetwork(name string) dockercompat.Network {
	cmdResult := b.Cmd("network", "inspect", name).Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var dc []dockercompat.Network
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &dc); err != nil {
		b.T.Fatal(err)
	}
	assert.Equal(b.T, 1, len(dc))
	return dc[0]
}

func (b *Base) InspectVolume(name string, args ...string) native.Volume {
	cmd := append([]string{"volume", "inspect"}, args...)
	cmd = append(cmd, name)
	cmdResult := b.Cmd(cmd...).Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var dc []native.Volume
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &dc); err != nil {
		b.T.Fatal(err)
	}
	assert.Equal(b.T, 1, len(dc))
	return dc[0]
}

func (b *Base) Info() dockercompat.Info {
	cmdResult := b.Cmd("info", "--format", "{{ json . }}").Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var info dockercompat.Info
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &info); err != nil {
		b.T.Fatal(err)
	}
	return info
}

func (b *Base) InfoNative() native.Info {
	b.T.Helper()
	if IsDocker() {
		b.T.Skip("InfoNative() should not be called for non-nerdctl target")
	}
	cmdResult := b.Cmd("info", "--mode", "native", "--format", "{{ json . }}").Run()
	assert.Equal(b.T, cmdResult.ExitCode, 0)
	var info native.Info
	if err := json.Unmarshal([]byte(cmdResult.Stdout()), &info); err != nil {
		b.T.Fatal(err)
	}
	return info
}

func (b *Base) ContainerdAddress() string {
	b.T.Helper()
	if IsDocker() {
		b.T.Skip("ContainerdAddress() should not be called for non-nerdctl target")
	}
	if os.Geteuid() == 0 {
		return defaults.DefaultAddress
	}
	xdr, err := rootlessutil.XDGRuntimeDir()
	if err != nil {
		b.T.Log(err)
		xdr = fmt.Sprintf("/run/user/%d", os.Geteuid())
	}
	pidFile := filepath.Join(xdr, "containerd-rootless", "child_pid")
	pidB, err := os.ReadFile(pidFile)
	if err != nil {
		b.T.Fatal(err)
	}
	pidS := strings.TrimSpace(string(pidB))
	return filepath.Join("/proc", pidS, "root", defaults.DefaultAddress)
}

func (b *Base) EnsureContainerStarted(con string) {
	b.T.Helper()

	const (
		maxRetry = 5
		sleep    = time.Second
	)
	for i := 0; i < maxRetry; i++ {
		if b.InspectContainer(con).State.Running {
			b.T.Logf("container %s is now running", con)
			return
		}
		b.T.Logf("(retry=%d)", i+1)
		time.Sleep(sleep)
	}
	b.T.Fatalf("conainer %s not running", con)
}

func (b *Base) EnsureContainerExited(con string, expectedExitCode int) {
	b.T.Helper()

	const (
		maxRetry = 5
		sleep    = time.Second
	)
	var c dockercompat.Container
	for i := 0; i < maxRetry; i++ {
		c = b.InspectContainer(con)
		if c.State.Status == "exited" {
			b.T.Logf("container %s have exited with status %d", con, c.State.ExitCode)
			if c.State.ExitCode == expectedExitCode {
				return
			}
			break
		}
		b.T.Logf("(retry=%d)", i+1)
		time.Sleep(sleep)
	}
	b.T.Fatalf("expected conainer %s to have exited with code %d, got status %+v",
		con, expectedExitCode, c.State)
}

type Cmd struct {
	icmd.Cmd
	*Base
	runResult *icmd.Result
	mu        sync.Mutex
}

func (c *Cmd) Run() *icmd.Result {
	c.Base.T.Helper()
	c.mu.Lock()
	c.runResult = icmd.RunCmd(c.Cmd)
	c.mu.Unlock()
	return c.runResult
}

func (c *Cmd) runIfNecessary() *icmd.Result {
	c.Base.T.Helper()
	c.mu.Lock()
	if c.runResult == nil {
		c.runResult = icmd.RunCmd(c.Cmd)
	}
	c.mu.Unlock()
	return c.runResult
}

func (c *Cmd) Start() *icmd.Result {
	c.Base.T.Helper()
	return icmd.StartCmd(c.Cmd)
}

func (c *Cmd) CmdOption(cmdOptions ...func(*Cmd)) *Cmd {
	for _, opt := range cmdOptions {
		opt(c)
	}
	return c
}

func (c *Cmd) Assert(expected icmd.Expected) {
	c.Base.T.Helper()
	c.runIfNecessary().Assert(c.Base.T, expected)
}

func (c *Cmd) AssertOK() {
	c.Base.T.Helper()
	c.AssertExitCode(0)
}

func (c *Cmd) AssertFail() {
	c.Base.T.Helper()
	res := c.runIfNecessary()
	assert.Assert(c.Base.T, res.ExitCode != 0, res)
}

func (c *Cmd) AssertExitCode(exitCode int) {
	c.Base.T.Helper()
	res := c.runIfNecessary()
	assert.Assert(c.Base.T, res.ExitCode == exitCode, res)
}

func (c *Cmd) AssertOutContains(s string) {
	c.Base.T.Helper()
	expected := icmd.Expected{
		Out: s,
	}
	c.Assert(expected)
}

func (c *Cmd) AssertCombinedOutContains(s string) {
	c.Base.T.Helper()
	res := c.runIfNecessary()
	assert.Assert(c.Base.T, strings.Contains(res.Combined(), s), fmt.Sprintf("expected output to contain %q: %q", s, res.Combined()))
}

// AssertOutContainsAll checks if command output contains All strings in `strs`.
func (c *Cmd) AssertOutContainsAll(strs ...string) {
	c.Base.T.Helper()
	fn := func(stdout string) error {
		for _, s := range strs {
			if !strings.Contains(stdout, s) {
				return fmt.Errorf("expected stdout to contain %q", s)
			}
		}
		return nil
	}
	c.AssertOutWithFunc(fn)
}

// AssertOutContainsAny checks if command output contains Any string in `strs`.
func (c *Cmd) AssertOutContainsAny(strs ...string) {
	c.Base.T.Helper()
	fn := func(stdout string) error {
		for _, s := range strs {
			if strings.Contains(stdout, s) {
				return nil
			}
		}
		return fmt.Errorf("expected stdout to contain any of %q", strings.Join(strs, "|"))
	}
	c.AssertOutWithFunc(fn)
}

func (c *Cmd) AssertOutNotContains(s string) {
	c.Base.T.Helper()
	c.AssertOutWithFunc(func(stdout string) error {
		if strings.Contains(stdout, s) {
			return fmt.Errorf("expected stdout to not contain %q", s)
		}
		return nil
	})
}

func (c *Cmd) AssertOutExactly(s string) {
	c.Base.T.Helper()
	fn := func(stdout string) error {
		if stdout != s {
			return fmt.Errorf("expected %q, got %q", s, stdout)
		}
		return nil
	}
	c.AssertOutWithFunc(fn)
}

func (c *Cmd) AssertOutWithFunc(fn func(stdout string) error) {
	c.Base.T.Helper()
	res := c.runIfNecessary()
	assert.Equal(c.Base.T, 0, res.ExitCode, res)
	assert.NilError(c.Base.T, fn(res.Stdout()), res.Combined())
}

func (c *Cmd) Out() string {
	c.Base.T.Helper()
	res := c.runIfNecessary()
	assert.Equal(c.Base.T, 0, res.ExitCode, res)
	return res.Stdout()
}

var (
	flagTestTarget     string
	flagTestKillDaemon bool
	flagTestIPv6       bool
	flagTestKube       bool
	flagTestFlaky      bool
)

var (
	testLockFile = filepath.Join(os.TempDir(), "nerdctl-test-prevent-concurrency", ".lock")
)

func M(m *testing.M) {
	flag.StringVar(&flagTestTarget, "test.target", "nerdctl", "target to test")
	flag.BoolVar(&flagTestKillDaemon, "test.allow-kill-daemon", false, "enable tests that kill the daemon")
	flag.BoolVar(&flagTestIPv6, "test.only-ipv6", false, "enable tests on IPv6")
	flag.BoolVar(&flagTestKube, "test.only-kubernetes", false, "enable tests on Kubernetes")
	flag.BoolVar(&flagTestFlaky, "test.only-flaky", false, "enable testing of flaky tests only (if false, flaky tests are ignored)")
	flag.Parse()

	if flagTestTarget == "" {
		flagTestTarget = "nerdctl"
	}

	os.Exit(func() int {
		// If there is a lockfile (no err), or if we error-ed stating it (permission), another test run is currently going.
		// Note that this could be racy. The .lock file COULD get acquired after this and before we hit the lock section.
		// This is not a big deal then: we will just wait for the lock to free.
		if _, err := os.Stat(testLockFile); err == nil || !errors.Is(err, os.ErrNotExist) {
			log.L.Errorf("Another test binary is already running. If you think this is an error, manually remove %s", testLockFile)
			return 1
		}

		err := os.MkdirAll(filepath.Dir(testLockFile), 0o777)
		if err != nil {
			log.L.WithError(err).Errorf("failed creating testing lock directory %q", filepath.Dir(testLockFile))
			return 1
		}

		// Ensure that permissions are set to 777 (regardless of umask value), so that we do not lock people out when
		// switching between rootful and rootless locking
		os.Chmod(filepath.Dir(testLockFile), 0o777)

		// Acquire lock
		lock, err := lockutil.Lock(filepath.Dir(testLockFile))
		if err != nil {
			log.L.WithError(err).Errorf("failed acquiring testing lock %q", filepath.Dir(testLockFile))
			return 1
		}

		// Release...
		defer lockutil.Unlock(lock)

		// Create marker file
		err = os.WriteFile(testLockFile, []byte("prevent testing from running in parallel for subpackages integration tests"), 0o666)
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

func IsDocker() bool {
	return strings.HasPrefix(filepath.Base(GetTarget()), "docker")
}

func DockerIncompatible(t testing.TB) {
	if IsDocker() {
		t.Skip("test is incompatible with Docker")
	}
}

func RequiresBuild(t testing.TB) {
	if !IsDocker() {
		buildkitHost, err := buildkitutil.GetBuildkitHost(Namespace)
		if err != nil {
			t.Skipf("test requires buildkitd: %+v", err)
		}
		t.Logf("buildkitHost=%q", buildkitHost)
	}
}

func RequireExecPlatform(t testing.TB, ss ...string) {
	ok, err := platformutil.CanExecProbably(ss...)
	if !ok {
		msg := fmt.Sprintf("test requires platform %v", ss)
		if err != nil {
			msg += fmt.Sprintf(": %v", err)
		}
		t.Skip(msg)
	}
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

func RequireContainerdPlugin(base *Base, requiredType, requiredID string, requiredCaps []string) {
	base.T.Helper()
	info := base.InfoNative()
	for _, p := range info.Daemon.Plugins.Plugins {
		if p.Type != requiredType {
			continue
		}
		if p.ID != requiredID {
			continue
		}
		pCapMap := make(map[string]struct{}, len(p.Capabilities))
		for _, f := range p.Capabilities {
			pCapMap[f] = struct{}{}
		}
		for _, f := range requiredCaps {
			if _, ok := pCapMap[f]; !ok {
				base.T.Skipf("test requires containerd plugin \"%s.%s\" with capabilities %v (missing %q)", requiredType, requiredID, requiredCaps, f)
			}
		}
		return
	}
	if len(requiredCaps) == 0 {
		base.T.Skipf("test requires containerd plugin \"%s.%s\"", requiredType, requiredID)
	} else {
		base.T.Skipf("test requires containerd plugin \"%s.%s\" with capabilities %v", requiredType, requiredID, requiredCaps)
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

// RequireExecutable skips tests when executable `name` is not present in PATH.
func RequireExecutable(t testing.TB, name string) {
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("required executable doesn't exist in PATH: %s", name)
	}
}

const Namespace = "nerdctl-test"

func NewBaseWithNamespace(t *testing.T, ns string) *Base {
	if ns == "" || ns == "default" || ns == Namespace {
		t.Fatalf(`the other base namespace cannot be "%s"`, ns)
	}
	return newBase(t, ns, false, false)
}

func NewBaseWithIPv6Compatible(t *testing.T) *Base {
	return newBase(t, Namespace, true, false)
}

func NewBase(t *testing.T) *Base {
	return newBase(t, Namespace, false, false)
}

func newBase(t *testing.T, ns string, ipv6Compatible bool, kubernetesCompatible bool) *Base {
	base := &Base{
		T:                    t,
		Target:               GetTarget(),
		DaemonIsKillable:     GetDaemonIsKillable(),
		EnableIPv6:           GetEnableIPv6(),
		IPv6Compatible:       ipv6Compatible,
		EnableKubernetes:     GetEnableKubernetes(),
		KubernetesCompatible: kubernetesCompatible,
		Env:                  os.Environ(),
	}
	if base.EnableIPv6 && !base.IPv6Compatible {
		t.Skip("runner skips non-IPv6 compatible tests in the IPv6 environment")
	} else if !base.EnableIPv6 && base.IPv6Compatible {
		t.Skip("runner skips IPv6 compatible tests in the non-IPv6 environment")
	}
	if base.EnableKubernetes && !base.KubernetesCompatible {
		t.Skip("runner skips non-Kubernetes compatible tests in the Kubernetes environment")
	} else if !base.EnableKubernetes && base.KubernetesCompatible {
		t.Skip("runner skips Kubernetes compatible tests in the non-Kubernetes environment")
	}
	if !GetFlakyEnvironment() && !GetEnableKubernetes() && !GetEnableIPv6() {
		t.Skip("legacy tests are considered flaky by default and are skipped unless in the flaky environment")
	}
	var err error
	base.Binary, err = exec.LookPath(base.Target)
	if err != nil {
		t.Fatal(err)
	}

	if IsDocker() {
		if err = exec.Command(base.Binary, "compose", "version").Run(); err != nil {
			t.Fatalf("docker does not support compose: %v", err)
		}
	} else {
		base.Args = []string{"--namespace=" + ns}
	}

	return base
}

// Identifier can be used as a name of container, image, volume, network, etc.
func Identifier(t testing.TB) string {
	s := t.Name()
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ToLower(s)
	s = "nerdctl-" + s
	if len(s) > 76 {
		s = "nerdctl-" + digest.SHA256.FromString(t.Name()).Encoded()
	}
	return s
}

// ImageRepo returns the image repo that can be used to, e.g, validate output
// from `nerdctl images`.
func ImageRepo(s string) string {
	repo, _ := imgutil.ParseRepoTag(s)
	return repo
}

// RegisterBuildCacheCleanup adds a 'builder prune --all --force' cleanup function
// to run on test teardown.
func RegisterBuildCacheCleanup(t *testing.T) {
	t.Cleanup(func() {
		NewBase(t).Cmd("builder", "prune", "--all", "--force").Run()
	})
}

func mirrorOf(s string) string {
	// plain mirror, NOT stargz-converted images
	return fmt.Sprintf("ghcr.io/stargz-containers/%s-org", s)
}
