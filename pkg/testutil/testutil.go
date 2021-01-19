/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"testing"

	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

type Base struct {
	T      testing.TB
	Target Target
	Binary string
	Args   []string
}

func (b *Base) Cmd(args ...string) *Cmd {
	icmdCmd := icmd.Command(b.Binary, append(b.Args, args...)...)
	cmd := &Cmd{
		Cmd:  icmdCmd,
		Base: b,
	}
	return cmd
}

type Cmd struct {
	icmd.Cmd
	*Base
	DockerIncompatible bool
}

func (c *Cmd) Run() *icmd.Result {
	if c.Base.Target == Docker && c.DockerIncompatible {
		c.Base.T.Skip("test is incompatible with Docker")
	}
	return icmd.RunCmd(c.Cmd)
}

func (c *Cmd) Assert(expected icmd.Expected) {
	c.Run().Assert(c.Base.T, expected)
}

func (c *Cmd) AssertOK() {
	expected := icmd.Expected{}
	c.Assert(expected)
}

func (c *Cmd) AssertOut(s string) {
	expected := icmd.Expected{
		Out: s,
	}
	c.Assert(expected)
}

func (c *Cmd) AssertOutWithFunc(fn func(stdout string) error) {
	res := c.Run()
	assert.Equal(c.Base.T, 0, res.ExitCode, res.Combined())
	assert.NilError(c.Base.T, fn(res.Stdout()), res.Combined())
}

type Target = string

const (
	Nerdctl = Target("nerdctl")
	Docker  = Target("docker")
)

var flagTestTarget Target

func M(m *testing.M) {
	flag.StringVar(&flagTestTarget, "test.target", Nerdctl, "target to test")
	flag.Parse()
	fmt.Printf("test target: %q\n", flagTestTarget)
	os.Exit(m.Run())
}

func GetTarget() string {
	if flagTestTarget == "" {
		panic("GetTarget() was called without calling M()")
	}
	return flagTestTarget
}

const Namespace = "nerdctl-test"

func NewBase(t *testing.T) *Base {
	if os.Geteuid() != 0 {
		t.Skip("test requires root")
	}
	base := &Base{
		T:      t,
		Target: GetTarget(),
	}
	var err error
	switch base.Target {
	case Nerdctl:
		base.Binary, err = exec.LookPath("nerdctl")
		if err != nil {
			t.Fatal(err)
		}
		base.Args = []string{"--namespace=" + Namespace}
	case Docker:
		base.Binary, err = exec.LookPath("docker")
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown test target %q", base.Target)
	}
	return base
}

// TODO: avoid using Docker Hub
const AlpineImage = "alpine"
