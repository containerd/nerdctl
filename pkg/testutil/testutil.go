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
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/nerdctl/pkg/buildkitutil"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/opencontainers/go-digest"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

type Base struct {
	T                testing.TB
	Target           Target
	DaemonIsKillable bool
	Binary           string
	ComposeBinary    string // "docker-compose"
	Args             []string
	Env              []string
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
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

// ComposeCmd executes `nerdctl -n nerdctl-test compose` or `docker-compose`
func (b *Base) ComposeCmd(args ...string) *Cmd {
	var (
		binary     string
		binaryArgs []string
	)
	if b.ComposeBinary != "" {
		binary = b.ComposeBinary
		binaryArgs = append(b.Args, args...)
	} else {
		binary = b.Binary
		binaryArgs = append(b.Args, append([]string{"compose"}, args...)...)
	}
	icmdCmd := icmd.Command(binary, binaryArgs...)
	icmdCmd.Env = b.Env
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
	helperArgs := helper[1:]
	var (
		binary     string
		binaryArgs []string
	)
	if b.ComposeBinary != "" {
		binary = b.ComposeBinary
		binaryArgs = append(b.Args, args...)
	} else {
		binary = b.Binary
		binaryArgs = append(b.Args, append([]string{"compose"}, args...)...)
	}
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
	switch b.Target {
	case Nerdctl:
		return "containerd.service"
	case Docker:
		return "docker.service"
	default:
		b.T.Fatalf("unexpected target %q", b.Target)
		return ""
	}
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
		b.T.Skip("daemon is not killable (hint: set \"-test.kill-daemon\")")
	}
	target := b.systemctlTarget()
	b.T.Logf("killing %q", target)
	cmdKill := exec.Command("systemctl",
		append(b.systemctlArgs(),
			[]string{"kill", "-s", "KILL", target}...)...)
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
		cmd := exec.Command("systemctl",
			append(systemctlArgs,
				[]string{"is-active", target}...)...)
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
		append(b.systemctlArgs(),
			[]string{"-u", target,
				"--no-pager",
				"-S", fmt.Sprintf("%d min ago", minutes)}...)...)
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
	if GetTarget() != Nerdctl {
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
	if GetTarget() != Nerdctl {
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

type Cmd struct {
	icmd.Cmd
	*Base
}

func (c *Cmd) Run() *icmd.Result {
	c.Base.T.Helper()
	return icmd.RunCmd(c.Cmd)
}

func (c *Cmd) CmdOption(cmdOptions ...func(*Cmd)) *Cmd {
	for _, opt := range cmdOptions {
		opt(c)
	}
	return c
}

func (c *Cmd) Assert(expected icmd.Expected) {
	c.Base.T.Helper()
	c.Run().Assert(c.Base.T, expected)
}

func (c *Cmd) AssertOK() {
	c.Base.T.Helper()
	c.AssertExitCode(0)
}

func (c *Cmd) AssertFail() {
	c.Base.T.Helper()
	res := c.Run()
	assert.Assert(c.Base.T, res.ExitCode != 0)
}

func (c *Cmd) AssertExitCode(exitCode int) {
	c.Base.T.Helper()
	res := c.Run()
	assert.Assert(c.Base.T, res.ExitCode == exitCode, res.Combined())
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
	res := c.Run()
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

func (c *Cmd) AssertOutStreamsExactly(stdout, stderr string) {
	c.Base.T.Helper()
	fn := func(sout, serr string) error {
		msg := ""
		if sout != stdout {
			msg += fmt.Sprintf("stdout mismatch, expected %q, got %q\n", stdout, sout)
		}
		if serr != stderr {
			msg += fmt.Sprintf("stderr mismatch, expected %q, got %q\n", stderr, serr)
		}
		if msg != "" {
			return fmt.Errorf(msg)
		}
		return nil
	}
	c.AssertOutStreamsWithFunc(fn)
}

func (c *Cmd) AssertNoOut(s string) {
	c.Base.T.Helper()
	fn := func(stdout string) error {
		if strings.Contains(stdout, s) {
			return fmt.Errorf("expected not to contain %q, got %q", s, stdout)
		}
		return nil
	}
	c.AssertOutWithFunc(fn)
}

func (c *Cmd) AssertOutWithFunc(fn func(stdout string) error) {
	c.Base.T.Helper()
	res := c.Run()
	assert.Equal(c.Base.T, 0, res.ExitCode, res.Combined())
	assert.NilError(c.Base.T, fn(res.Stdout()), res.Combined())
}

func (c *Cmd) AssertOutStreamsWithFunc(fn func(stdout, stderr string) error) {
	c.Base.T.Helper()
	res := c.Run()
	assert.Equal(c.Base.T, 0, res.ExitCode, res.Combined())
	assert.NilError(c.Base.T, fn(res.Stdout(), res.Stderr()), res.Combined())
}

func (c *Cmd) Out() string {
	c.Base.T.Helper()
	res := c.Run()
	assert.Equal(c.Base.T, 0, res.ExitCode, res.Combined())
	return res.Stdout()
}

func (c *Cmd) OutLines() []string {
	c.Base.T.Helper()
	out := c.Out()
	// FIXME: improve memory efficiency
	return strings.Split(out, "\n")
}

type Target = string

const (
	Nerdctl = Target("nerdctl")
	Docker  = Target("docker")
)

var (
	flagTestTarget     Target
	flagTestKillDaemon bool
)

func M(m *testing.M) {
	flag.StringVar(&flagTestTarget, "test.target", Nerdctl, "target to test")
	flag.BoolVar(&flagTestKillDaemon, "test.kill-daemon", false, "enable tests that kill the daemon")
	flag.Parse()
	fmt.Fprintf(os.Stderr, "test target: %q\n", flagTestTarget)
	os.Exit(m.Run())
}

func GetTarget() string {
	if flagTestTarget == "" {
		panic("GetTarget() was called without calling M()")
	}
	return flagTestTarget
}

func GetDaemonIsKillable() bool {
	return flagTestKillDaemon
}

func DockerIncompatible(t testing.TB) {
	if GetTarget() == Docker {
		t.Skip("test is incompatible with Docker")
	}
}

func RequiresBuild(t testing.TB) {
	if GetTarget() == Nerdctl {
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

func RequireDaemonVersion(b *Base, constraint string) {
	b.T.Helper()
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		b.T.Fatal(err)
	}
	info := b.Info()
	sv, err := semver.NewVersion(info.ServerVersion)
	if err != nil {
		b.T.Skip(err)
	}
	if !c.Check(sv) {
		b.T.Skipf("version %v does not satisfy constraints %v", sv, c)
	}
}

func RequireKernelVersion(t testing.TB, constraint string) {
	t.Helper()
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		t.Fatal(err)
	}
	unameR, err := semver.NewVersion(infoutil.UnameR())
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
	return newBase(t, ns)
}

func NewBase(t *testing.T) *Base {
	return newBase(t, Namespace)
}

func newBase(t *testing.T, ns string) *Base {
	base := &Base{
		T:                t,
		Target:           GetTarget(),
		DaemonIsKillable: GetDaemonIsKillable(),
	}
	var err error
	switch base.Target {
	case Nerdctl:
		base.Binary, err = exec.LookPath("nerdctl")
		if err != nil {
			t.Fatal(err)
		}
		base.Args = []string{"--namespace=" + ns}
		base.ComposeBinary = ""
	case Docker:
		base.Binary, err = exec.LookPath("docker")
		if err != nil {
			t.Fatal(err)
		}
		base.ComposeBinary, err = exec.LookPath("docker-compose")
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown test target %q", base.Target)
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
