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

package container

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	syslog "github.com/yuchanns/srslog"

	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testsyslog"
)

// syslogCombination holds one entry of the (network x facility x format) cross product.
type syslogCombination struct {
	network      string
	fPriK        string
	fPriV        syslog.Priority
	fmtK         string
	fmtValidFunc func(string, string, string, string, syslog.Priority, bool) error
}

// buildSyslogSubTests expands the (network x facility x format) cross product
// into independent Tigron sub-cases. Each sub-case starts its own syslog
// listener in Command (immediately before the container launch) to avoid the
// 300ms goroutine timeout in runPacketSyslog expiring before the container
// sends its first log entry. Validation happens in Cleanup.
func buildSyslogSubTests(
	networks []string,
	syslogFacilities map[string]syslog.Priority,
	fmtValidFuncs map[string]func(string, string, string, string, syslog.Priority, bool) error,
	caRef **testca.CA,
	certRef **testca.Cert,
	hostnameRef *string,
) []*test.Case {
	var combinations []syslogCombination

	for _, network := range networks {
		for rFK, rFV := range syslogFacilities {
			for _, fPriK := range []string{rFK, strconv.Itoa(int(rFV) >> 3)} {
				for fmtK, fmtValidFunc := range fmtValidFuncs {
					combinations = append(combinations, syslogCombination{
						network:      network,
						fPriK:        fPriK,
						fPriV:        rFV,
						fmtK:         fmtK,
						fmtValidFunc: fmtValidFunc,
					})
				}
			}
		}
	}

	var cases []*test.Case

	for _, c := range combinations {
		fmtKT := "empty"
		if c.fmtK != "" {
			fmtKT = c.fmtK
		}
		subName := fmt.Sprintf("%s_%s_%s", strings.ReplaceAll(c.network, "+", "_"), c.fPriK, fmtKT)

		var (
			addr          string
			done          chan string
			closer        io.Closer
			containerName string
			tag           string
			msg           string
		)

		cases = append(cases, &test.Case{
			Description: subName,
			// runPacketSyslog reads with 4x100ms deadlines (~400ms total).
			// Parallel execution on slow arm runners pushes container startup
			// past that window, causing the server to send "" on the channel.
			// Sequential matches the original t.Run-based test behaviour.
			NoParallel: true,
			Setup: func(data test.Data, helpers test.Helpers) {
				if !testsyslog.TestableNetwork(c.network) {
					if rootlessutil.IsRootless() {
						helpers.T().Skip(fmt.Sprintf("%q for rootless containers is not supported", c.network))
					}
					helpers.T().Skip(fmt.Sprintf("%q is not supported", c.network))
				}
				tID := data.Identifier()
				tag = tID + "_syslog_driver"
				msg = "hello, " + tID + "_syslog_driver"
				containerName = fmt.Sprintf("%s-%s", tID, c.fPriK)
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				// Start the server here, immediately before launching the
				// container, so the 300ms goroutine timeout in
				// runPacketSyslog does not expire before the container
				// produces its first log entry.
				done = make(chan string)
				addr, closer = testsyslog.StartServer(c.network, "", done, *certRef)
				args := []string{
					"run",
					"-d",
					"--name", containerName,
					"--restart=no",
					"--log-driver=syslog",
					"--log-opt=syslog-facility=" + c.fPriK,
					"--log-opt=tag=" + tag,
					"--log-opt=syslog-format=" + c.fmtK,
					"--log-opt=syslog-address=" + fmt.Sprintf("%s://%s", c.network, addr),
				}
				if c.network == "tcp+tls" {
					cert := *certRef
					ca := *caRef
					args = append(args,
						"--log-opt=syslog-tls-cert="+cert.CertPath,
						"--log-opt=syslog-tls-key="+cert.KeyPath,
						"--log-opt=syslog-tls-ca-cert="+ca.CertPath,
					)
				}
				args = append(args, testutil.CommonImage, "echo", msg)
				return helpers.Command(args...)
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
			Cleanup: func(data test.Data, helpers test.Helpers) {
				if containerName != "" {
					helpers.Anyhow("rm", "-f", containerName)
				}
				if closer == nil || done == nil {
					return
				}
				defer closer.Close()
				defer close(done)
				select {
				case rcvd := <-done:
					if err := c.fmtValidFunc(rcvd, msg, tag, *hostnameRef, c.fPriV, c.network == "tcp+tls"); err != nil {
						helpers.T().Log(err)
						helpers.T().Fail()
					}
				case <-time.After(time.Second * 3):
					helpers.T().Log(fmt.Sprintf("timeout with %s", subName))
					helpers.T().Fail()
				}
			},
		})
	}

	return cases
}

// newSyslogTestCase wires the shared outer fixture: skip on Windows, pull the
// image, generate a CA/cert pair, and expose them to the sub-cases via the
// returned pointers.
func newSyslogTestCase(t *testing.T) (*test.Case, **testca.CA, **testca.Cert, *string) {
	t.Helper()

	testCase := &test.Case{
		Require: require.Not(require.OS("windows")),
	}

	var (
		ca       *testca.CA
		cert     *testca.Cert
		hostname string
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("pull", "--quiet", testutil.CommonImage)
		hn, err := os.Hostname()
		if err != nil {
			helpers.T().Log(fmt.Sprintf("retrieving hostname: %v", err))
			helpers.T().FailNow()
		}
		hostname = hn
		ca = testca.New(t)
		cert = ca.NewCert("127.0.0.1")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if cert != nil {
			cert.Close()
		}
		if ca != nil {
			ca.Close()
		}
	}

	return testCase, &ca, &cert, &hostname
}

func TestSyslogNetwork(t *testing.T) {
	base := nerdtest.Setup()
	tc, caRef, certRef, hostnameRef := newSyslogTestCase(t)
	base.Require = tc.Require
	base.Setup = tc.Setup
	base.Cleanup = tc.Cleanup

	syslogFacilities := map[string]syslog.Priority{
		"user": syslog.LOG_USER,
	}
	networks := []string{
		"udp",
		"tcp",
		"tcp+tls",
		"unix",
		"unixgram",
	}
	fmtValidFuncs := map[string]func(string, string, string, string, syslog.Priority, bool) error{
		"rfc5424": rfc5424Validator,
	}

	base.SubTests = buildSyslogSubTests(networks, syslogFacilities, fmtValidFuncs, caRef, certRef, hostnameRef)

	base.Run(t)
}

func TestSyslogFacilities(t *testing.T) {
	base := nerdtest.Setup()
	tc, caRef, certRef, hostnameRef := newSyslogTestCase(t)
	base.Require = tc.Require
	base.Setup = tc.Setup
	base.Cleanup = tc.Cleanup

	syslogFacilities := map[string]syslog.Priority{
		"kern":     syslog.LOG_KERN,
		"user":     syslog.LOG_USER,
		"mail":     syslog.LOG_MAIL,
		"daemon":   syslog.LOG_DAEMON,
		"auth":     syslog.LOG_AUTH,
		"syslog":   syslog.LOG_SYSLOG,
		"lpr":      syslog.LOG_LPR,
		"news":     syslog.LOG_NEWS,
		"uucp":     syslog.LOG_UUCP,
		"cron":     syslog.LOG_CRON,
		"authpriv": syslog.LOG_AUTHPRIV,
		"ftp":      syslog.LOG_FTP,
		"local0":   syslog.LOG_LOCAL0,
		"local1":   syslog.LOG_LOCAL1,
		"local2":   syslog.LOG_LOCAL2,
		"local3":   syslog.LOG_LOCAL3,
		"local4":   syslog.LOG_LOCAL4,
		"local5":   syslog.LOG_LOCAL5,
		"local6":   syslog.LOG_LOCAL6,
		"local7":   syslog.LOG_LOCAL7,
	}
	networks := []string{"unix"}
	fmtValidFuncs := map[string]func(string, string, string, string, syslog.Priority, bool) error{
		"rfc5424": rfc5424Validator,
	}

	base.SubTests = buildSyslogSubTests(networks, syslogFacilities, fmtValidFuncs, caRef, certRef, hostnameRef)

	base.Run(t)
}

func TestSyslogFormat(t *testing.T) {
	base := nerdtest.Setup()
	tc, caRef, certRef, hostnameRef := newSyslogTestCase(t)
	base.Require = tc.Require
	base.Setup = tc.Setup
	base.Cleanup = tc.Cleanup

	syslogFacilities := map[string]syslog.Priority{
		"user": syslog.LOG_USER,
	}
	networks := []string{"unix"}
	fmtValidFuncs := map[string]func(string, string, string, string, syslog.Priority, bool) error{
		"":             emptyFormatValidator,
		"rfc3164":      rfc3164Validator,
		"rfc5424":      rfc5424Validator,
		"rfc5424micro": rfc5424Validator,
	}

	base.SubTests = buildSyslogSubTests(networks, syslogFacilities, fmtValidFuncs, caRef, certRef, hostnameRef)

	base.Run(t)
}

func rfc5424Validator(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
	var parsedHostname, timestamp string
	var length, version, pid int
	if !isTLS {
		exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
		if n, err := fmt.Sscanf(rcvd, exp, &version, &timestamp, &parsedHostname, &pid); n != 4 || err != nil || hostname != parsedHostname {
			return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
		}
		return nil
	}
	exp := "%d " + fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
	if n, err := fmt.Sscanf(rcvd, exp, &length, &version, &timestamp, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
		return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
	}
	return nil
}

func rfc3164Validator(rcvd, msg, tag, hostname string, pri syslog.Priority, _ bool) error {
	var parsedHostname, mon, day, hrs string
	var pid int
	exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%s %s %s %s " + tag + "[%d]: " + msg + "\n"
	if n, err := fmt.Sscanf(rcvd, exp, &mon, &day, &hrs, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
		return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
	}
	return nil
}

func emptyFormatValidator(rcvd, msg, tag, _ string, pri syslog.Priority, _ bool) error {
	var mon, day, hrs string
	var pid int
	exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%s %s %s " + tag + "[%d]: " + msg + "\n"
	if n, err := fmt.Sscanf(rcvd, exp, &mon, &day, &hrs, &pid); n != 4 || err != nil {
		return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
	}
	return nil
}
