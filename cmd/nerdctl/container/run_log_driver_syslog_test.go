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
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	syslog "github.com/yuchanns/srslog"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testca"
	"github.com/containerd/nerdctl/v2/pkg/testutil/testsyslog"
)

func runSyslogTest(t *testing.T, networks []string, syslogFacilities map[string]syslog.Priority, fmtValidFuncs map[string]func(string, string, string, string, syslog.Priority, bool) error) {
	if runtime.GOOS == "windows" {
		t.Skip("syslog container logging is not officially supported on Windows")
	}

	base := testutil.NewBase(t)
	base.Cmd("pull", testutil.CommonImage).AssertOK()
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatalf("Error retrieving hostname")
	}
	ca := testca.New(base.T)
	cert := ca.NewCert("127.0.0.1")
	t.Cleanup(func() {
		cert.Close()
		ca.Close()
	})
	rI := 0
	for _, network := range networks {
		for rFK, rFV := range syslogFacilities {
			fPriV := rFV
			// test both string and number facility
			for _, fPriK := range []string{rFK, strconv.Itoa(int(fPriV) >> 3)} {
				for fmtK, fmtValidFunc := range fmtValidFuncs {
					fmtKT := "empty"
					if fmtK != "" {
						fmtKT = fmtK
					}
					subTestName := fmt.Sprintf("%s_%s_%s", strings.ReplaceAll(network, "+", "_"), fPriK, fmtKT)
					i := rI
					rI++
					t.Run(subTestName, func(t *testing.T) {
						tID := testutil.Identifier(t)
						tag := tID + "_syslog_driver"
						msg := "hello, " + tID + "_syslog_driver"
						if !testsyslog.TestableNetwork(network) {
							if rootlessutil.IsRootless() {
								t.Skipf("skipping on %s/%s; '%s' for rootless containers are not supported", runtime.GOOS, runtime.GOARCH, network)
							}
							t.Skipf("skipping on %s/%s; '%s' is not supported", runtime.GOOS, runtime.GOARCH, network)
						}
						testContainerName := fmt.Sprintf("%s-%d-%s", tID, i, fPriK)
						done := make(chan string)
						addr, closer := testsyslog.StartServer(network, "", done, cert)
						args := []string{
							"run",
							"-d",
							"--name",
							testContainerName,
							"--restart=no",
							"--log-driver=syslog",
							"--log-opt=syslog-facility=" + fPriK,
							"--log-opt=tag=" + tag,
							"--log-opt=syslog-format=" + fmtK,
							"--log-opt=syslog-address=" + fmt.Sprintf("%s://%s", network, addr),
						}
						if network == "tcp+tls" {
							args = append(args,
								"--log-opt=syslog-tls-cert="+cert.CertPath,
								"--log-opt=syslog-tls-key="+cert.KeyPath,
								"--log-opt=syslog-tls-ca-cert="+ca.CertPath,
							)
						}
						args = append(args, testutil.CommonImage, "echo", msg)
						base.Cmd(args...).AssertOK()
						t.Cleanup(func() {
							base.Cmd("rm", "-f", testContainerName).AssertOK()
						})
						defer closer.Close()
						defer close(done)
						select {
						case rcvd := <-done:
							if err := fmtValidFunc(rcvd, msg, tag, hostname, fPriV, network == "tcp+tls"); err != nil {
								t.Error(err)
							}
						case <-time.Tick(time.Second * 3):
							t.Errorf("timeout with %s", subTestName)
						}
					})
				}
			}
		}
	}
}

func TestSyslogNetwork(t *testing.T) {
	var syslogFacilities = map[string]syslog.Priority{
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
		"rfc5424": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
			var parsedHostname, timestamp string
			var length, version, pid int
			if !isTLS {
				exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &version, &timestamp, &parsedHostname, &pid); n != 4 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			} else {
				exp := "%d " + fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &length, &version, &timestamp, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			}
			return nil
		},
	}
	runSyslogTest(t, networks, syslogFacilities, fmtValidFuncs)
}

func TestSyslogFacilities(t *testing.T) {
	var syslogFacilities = map[string]syslog.Priority{
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
		"rfc5424": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
			var parsedHostname, timestamp string
			var length, version, pid int
			if !isTLS {
				exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &version, &timestamp, &parsedHostname, &pid); n != 4 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			} else {
				exp := "%d " + fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &length, &version, &timestamp, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			}
			return nil
		},
	}
	runSyslogTest(t, networks, syslogFacilities, fmtValidFuncs)
}

func TestSyslogFormat(t *testing.T) {
	var syslogFacilities = map[string]syslog.Priority{
		"user": syslog.LOG_USER,
	}

	networks := []string{"unix"}
	fmtValidFuncs := map[string]func(string, string, string, string, syslog.Priority, bool) error{
		"": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isSTLS bool) error {
			var mon, day, hrs string
			var pid int
			exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%s %s %s " + tag + "[%d]: " + msg + "\n"
			if n, err := fmt.Sscanf(rcvd, exp, &mon, &day, &hrs, &pid); n != 4 || err != nil {
				return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
			}
			return nil
		},
		"rfc3164": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
			var parsedHostname, mon, day, hrs string
			var pid int
			exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%s %s %s %s " + tag + "[%d]: " + msg + "\n"
			if n, err := fmt.Sscanf(rcvd, exp, &mon, &day, &hrs, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
				return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
			}
			return nil
		},
		"rfc5424": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
			var parsedHostname, timestamp string
			var length, version, pid int
			if !isTLS {
				exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &version, &timestamp, &parsedHostname, &pid); n != 4 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			} else {
				exp := "%d " + fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &length, &version, &timestamp, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			}
			return nil
		},
		"rfc5424micro": func(rcvd, msg, tag, hostname string, pri syslog.Priority, isTLS bool) error {
			var parsedHostname, timestamp string
			var length, version, pid int
			if !isTLS {
				exp := fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &version, &timestamp, &parsedHostname, &pid); n != 4 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			} else {
				exp := "%d " + fmt.Sprintf("<%d>", pri|syslog.LOG_INFO) + "%d %s %s " + tag + " %d " + tag + " - " + msg + "\n"
				if n, err := fmt.Sscanf(rcvd, exp, &length, &version, &timestamp, &parsedHostname, &pid); n != 5 || err != nil || hostname != parsedHostname {
					return fmt.Errorf("s.Info() = '%q', didn't match '%q' (%d %s)", rcvd, exp, n, err)
				}
			}
			return nil
		},
	}
	runSyslogTest(t, networks, syslogFacilities, fmtValidFuncs)
}
