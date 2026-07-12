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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/opencontainers/go-digest"
	"github.com/vishvananda/netlink"
	"gotest.tools/v3/assert"

	"github.com/containerd/containerd/v2/defaults"
	"github.com/containerd/containerd/v2/pkg/netns"
	"github.com/containerd/nerdctl/mod/tigron/expect"
	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"
	"github.com/containerd/nerdctl/mod/tigron/tig"

	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

func extractHostPort(portMapping string, port string) (string, error) {
	// Regular expression to extract host port from port mapping information
	re := regexp.MustCompile(`(?P<containerPort>\d{1,5})/tcp ->.*?0.0.0.0:(?P<hostPort>\d{1,5}).*?`)
	portMappingLines := strings.Split(portMapping, "\n")
	for _, portMappingLine := range portMappingLines {
		// Find the matches
		matches := re.FindStringSubmatch(portMappingLine)
		// Check if there is a match
		if len(matches) >= 3 && matches[1] == port {
			// Extract the host port number
			hostPort := matches[2]
			return hostPort, nil
		}
	}
	return "", fmt.Errorf("could not extract host port from port mapping: %s", portMapping)
}

// TestRunInternetConnectivity tests Internet connectivity with `apk update`
func TestRunInternetConnectivity(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("network", "create", data.Identifier("customnet"))
		netw := nerdtest.InspectNetwork(helpers, data.Identifier("customnet"))
		data.Labels().Set("customNet", data.Identifier("customnet"))
		data.Labels().Set("customNetID", netw.ID)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Identifier("customnet"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "--net bridge",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", "bridge", testutil.AlpineImage, "apk", "update")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("OK")),
		},
		{
			Description: "--net customNet",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("customNet"), testutil.AlpineImage, "apk", "update")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("OK")),
		},
		{
			Description: "--net customNetID (full)",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("customNetID"), testutil.AlpineImage, "apk", "update")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("OK")),
		},
		{
			Description: "--net customNetID (short)",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", data.Labels().Get("customNetID")[:12], testutil.AlpineImage, "apk", "update")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("OK")),
		},
		{
			Description: "--net host",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--net", "host", testutil.AlpineImage, "apk", "update")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("OK")),
		},
	}

	testCase.Run(t)
}

// TestRunHostLookup tests hostname lookup
func TestRunHostLookup(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// key: container name suffix, val: network name
		m := map[string]string{
			"c0-in-n0":     data.Identifier("n0"),
			"c1-in-n0":     data.Identifier("n0"),
			"c2-in-n1":     data.Identifier("n1"),
			"c3-in-bridge": "bridge",
		}

		// Create networks
		helpers.Ensure("network", "create", data.Identifier("n0"))
		helpers.Ensure("network", "create", data.Identifier("n1"))

		// Store network and container names in labels
		data.Labels().Set("net-n0", data.Identifier("n0"))
		data.Labels().Set("net-n1", data.Identifier("n1"))

		// Create nginx containers
		for name, netName := range m {
			containerName := data.Identifier(name)
			data.Labels().Set(name, containerName)
			helpers.Ensure("run", "-d", "--name", containerName, "--hostname", name+"-foobar", "--net", netName, testutil.NginxAlpineImage)
		}
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		for _, name := range []string{"c0-in-n0", "c1-in-n0", "c2-in-n1", "c3-in-bridge"} {
			helpers.Anyhow("rm", "-f", data.Identifier(name))
		}
		helpers.Anyhow("network", "rm", data.Identifier("n0"))
		helpers.Anyhow("network", "rm", data.Identifier("n1"))
	}

	type wgetCase struct {
		srcSuffix     string
		buildTarget   func(data test.Data) string
		desc          string
		shouldSucceed bool
	}

	wgetCases := []wgetCase{
		{"c0-in-n0", func(d test.Data) string { return d.Labels().Get("c1-in-n0") }, "container name", true},
		{"c0-in-n0", func(d test.Data) string { return d.Labels().Get("c1-in-n0") + "." + d.Labels().Get("net-n0") }, "container FQDN", true},
		{"c0-in-n0", func(d test.Data) string { return "c1-in-n0-foobar" }, "hostname", true},
		{"c0-in-n0", func(d test.Data) string { return "c1-in-n0-foobar." + d.Labels().Get("net-n0") }, "hostname FQDN", true},
		{"c0-in-n0", func(d test.Data) string { return d.Labels().Get("c2-in-n1") }, "cross-network name", false},
		{"c0-in-n0", func(d test.Data) string { return d.Labels().Get("c2-in-n1") + "." + d.Labels().Get("net-n1") }, "cross-network FQDN", false},
		{"c0-in-n0", func(d test.Data) string { return d.Labels().Get("c3-in-bridge") }, "bridge container", false},
		{"c1-in-n0", func(d test.Data) string { return d.Labels().Get("c0-in-n0") }, "reverse container name", true},
		{"c1-in-n0", func(d test.Data) string { return d.Labels().Get("c0-in-n0") + "." + d.Labels().Get("net-n0") }, "reverse FQDN", true},
		{"c1-in-n0", func(d test.Data) string { return "c0-in-n0-foobar" }, "reverse hostname", true},
		{"c1-in-n0", func(d test.Data) string { return "c0-in-n0-foobar." + d.Labels().Get("net-n0") }, "reverse hostname FQDN", true},
	}

	testCase.SubTests = make([]*test.Case, 0, len(wgetCases))
	for _, wc := range wgetCases {
		wc := wc
		desc := fmt.Sprintf("%s from %s (expect %v)", wc.desc, wc.srcSuffix, wc.shouldSucceed)
		var expected test.Manager
		if wc.shouldSucceed {
			expected = test.Expects(expect.ExitCodeSuccess, nil, expect.Contains(testutil.NginxAlpineIndexHTMLSnippet))
		} else {
			expected = test.Expects(expect.ExitCodeGenericFail, nil, nil)
		}
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: desc,
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				target := wc.buildTarget(data)
				return helpers.Command("exec", data.Labels().Get(wc.srcSuffix), "wget", "-qO-", "http://"+target)
			},
			Expected: expected,
		})
	}

	testCase.Run(t)
}

func TestRunPortWithNoHostPort(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Rootful // Auto port assign is not supported rootless mode yet

	type portTestCase struct {
		containerPort string
	}
	testCases := []portTestCase{
		{containerPort: "80"},
		{containerPort: "80-81"},
		{containerPort: "80-81/tcp"},
	}

	for i, tc := range testCases {
		tc := tc
		i := i
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: fmt.Sprintf("port %s", tc.containerPort),
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				containerName := data.Identifier(fmt.Sprintf("container-%d", i))
				data.Labels().Set("containerName", containerName)
				helpers.Ensure("run", "-d", "--name", containerName, "-p", tc.containerPort, testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier(fmt.Sprintf("container-%d", i)))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("port", data.Labels().Get("containerName"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						regexExpression := regexp.MustCompile(`80\/tcp.*?->.*?0.0.0.0:(?P<portNumber>\d{1,5}).*?`)
						match := regexExpression.FindStringSubmatch(stdout)
						paramsMap := make(map[string]string)
						for j, name := range regexExpression.SubexpNames() {
							if j > 0 && j <= len(match) {
								paramsMap[name] = match[j]
							}
						}
						assert.Assert(t, paramsMap["portNumber"] != "", "could not extract port number from: %s", stdout)
						connectURL := fmt.Sprintf("http://%s:%s", "127.0.0.1", paramsMap["portNumber"])
						resp, err := nettestutil.HTTPGet(connectURL, 5, false)
						assert.NilError(t, err)
						respBody, err := io.ReadAll(resp.Body)
						assert.NilError(t, err)
						assert.Assert(t, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet))
					},
				}
			},
		})
	}

	testCase.Run(t)
}

func TestUniqueHostPortAssignement(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Rootful // Auto port assign is not supported rootless mode yet

	type portTestCase struct {
		containerPort string
	}
	testCases := []portTestCase{
		{containerPort: "80"},
		{containerPort: "80-81"},
		{containerPort: "80-81/tcp"},
	}

	for i, tc := range testCases {
		tc := tc
		i := i
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: fmt.Sprintf("port %s", tc.containerPort),
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				name1 := data.Identifier(fmt.Sprintf("c%d-1", i))
				name2 := data.Identifier(fmt.Sprintf("c%d-2", i))
				data.Labels().Set("container1", name1)
				data.Labels().Set("container2", name2)
				helpers.Ensure("run", "-d", "--name", name1, "-p", tc.containerPort, testutil.NginxAlpineImage)
				helpers.Ensure("run", "-d", "--name", name2, "-p", tc.containerPort, testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier(fmt.Sprintf("c%d-1", i)), data.Identifier(fmt.Sprintf("c%d-2", i)))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("port", data.Labels().Get("container1"))
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						port1, err := extractHostPort(stdout, "80")
						assert.NilError(t, err)

						// Get port for second container
						port2Stdout := helpers.Capture("port", data.Labels().Get("container2"))
						port2, err := extractHostPort(port2Stdout, "80")
						assert.NilError(t, err)

						assert.Assert(t, port1 != port2, "Host ports are not unique")

						// Make HTTP GET request to container 1
						connectURL1 := fmt.Sprintf("http://%s:%s", "127.0.0.1", port1)
						resp1, err := nettestutil.HTTPGet(connectURL1, 5, false)
						assert.NilError(t, err)
						respBody1, err := io.ReadAll(resp1.Body)
						assert.NilError(t, err)
						assert.Assert(t, strings.Contains(string(respBody1), testutil.NginxAlpineIndexHTMLSnippet))

						// Make HTTP GET request to container 2
						connectURL2 := fmt.Sprintf("http://%s:%s", "127.0.0.1", port2)
						resp2, err := nettestutil.HTTPGet(connectURL2, 5, false)
						assert.NilError(t, err)
						respBody2, err := io.ReadAll(resp2.Body)
						assert.NilError(t, err)
						assert.Assert(t, strings.Contains(string(respBody2), testutil.NginxAlpineIndexHTMLSnippet))
					},
				}
			},
		})
	}

	testCase.Run(t)
}

func TestHostPortAlreadyInUse(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.NoParallel = true

	type portConflictCase struct {
		hostPort      string
		containerPort string
	}
	testCases := []portConflictCase{
		{hostPort: "5000", containerPort: "80/tcp"},
		{hostPort: "5000", containerPort: "80/tcp"},
		{hostPort: "5000", containerPort: "80/udp"},
		{hostPort: "5000", containerPort: "80/sctp"},
	}

	for i, tc := range testCases {
		tc := tc
		i := i
		subTest := &test.Case{
			Description: fmt.Sprintf("%s:%s", tc.hostPort, tc.containerPort),
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				name1 := data.Identifier(fmt.Sprintf("c%d-1", i))
				data.Labels().Set("container1", name1)
				pFlag := fmt.Sprintf("%s:%s", tc.hostPort, tc.containerPort)
				helpers.Ensure("run", "-d", "--name", name1, "-p", pFlag, testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier(fmt.Sprintf("c%d-1", i)), data.Identifier(fmt.Sprintf("c%d-2", i)))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				name2 := data.Identifier(fmt.Sprintf("c%d-2", i))
				pFlag := fmt.Sprintf("%s:%s", tc.hostPort, tc.containerPort)
				return helpers.Command("run", "-d", "--name", name2, "-p", pFlag, testutil.NginxAlpineImage)
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		}
		if strings.Contains(tc.containerPort, "sctp") {
			subTest.Require = nerdtest.Rootful
		}
		testCase.SubTests = append(testCase.SubTests, subTest)
	}

	testCase.Run(t)
}

func TestRunPort(t *testing.T) {
	baseTestRunPort(t, testutil.NginxAlpineImage, testutil.NginxAlpineIndexHTMLSnippet, true)
}

func TestRunWithManyPortsThenCleanUp(t *testing.T) {
	testCase := nerdtest.Setup()
	// docker does not set label restriction to 4096 bytes
	testCase.Require = require.Not(nerdtest.Docker)

	testCase.SubTests = []*test.Case{
		{
			Description: "Run a container with many ports, and then clean up.",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--data-root", data.Temp().Path(), "--rm", "-p", "22200-22299:22200-22299", testutil.CommonImage)
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					ExitCode: 0,
					Errors:   []error{},
					Output: func(stdout string, t tig.T) {
						getAddrHash := func(addr string) string {
							const addrHashLen = 8

							d := digest.SHA256.FromString(addr)
							h := d.Encoded()[0:addrHashLen]

							return h
						}

						dataRoot := data.Temp().Path()
						h := getAddrHash(defaults.DefaultAddress)
						dataStore := filepath.Join(dataRoot, h)
						namespace := string(helpers.Read(nerdtest.Namespace))
						etchostsPath := filepath.Join(dataStore, "etchosts", namespace)

						etchostsDirs, err := os.ReadDir(etchostsPath)

						assert.NilError(t, err)
						assert.Equal(t, len(etchostsDirs), 0)
					},
				}
			},
		},
	}

	testCase.Run(t)
}

func TestRunContainerWithStaticIP(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = nerdtest.Rootful // Static IP assignment is not supported rootless mode yet

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("networkName", data.Identifier("network"))
		helpers.Ensure("network", "create", data.Identifier("network"), "--subnet", "172.0.0.0/16")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Identifier("network"))
	}

	// XXX see https://github.com/containerd/nerdctl/issues/3101
	// docker 24 silently ignored the ip - now, docker 26 is erroring out - furthermore, this ip only makes sense
	// in the context of nerdctl bridge network, so, this test needs rewritting either way
	testCase.SubTests = []*test.Case{
		{
			Description: "static IP within subnet succeeds",
			NoParallel:  true,
			Setup: func(data test.Data, helpers test.Helpers) {
				data.Labels().Set("containerName", data.Identifier("static-ip"))
				helpers.Ensure("run", "-d", "--name", data.Identifier("static-ip"), "--network", data.Labels().Get("networkName"), "--ip", "172.0.0.2", testutil.NginxAlpineImage)
			},
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("static-ip"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("containerName"),
					"--format", "{{range .NetworkSettings.Networks}} {{.IPAddress}}{{end}}")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("172.0.0.2")),
		},
		{
			Description: "static IP outside subnet fails",
			NoParallel:  true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				helpers.Anyhow("rm", "-f", data.Identifier("bad-ip"))
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "-d", "--name", data.Identifier("bad-ip"),
					"--network", data.Labels().Get("networkName"),
					"--ip", "192.0.0.2", testutil.NginxAlpineImage)
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestRunDNS(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "dns nameserver",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--dns", "8.8.8.8", testutil.CommonImage, "cat", "/etc/resolv.conf")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("nameserver 8.8.8.8\n")),
		},
		{
			Description: "dns search single",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--dns-search", "test", testutil.CommonImage, "cat", "/etc/resolv.conf")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("search test\n")),
		},
		{
			Description: "dns search multiple",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--dns-search", "test", "--dns-search", "test1", testutil.CommonImage, "cat", "/etc/resolv.conf")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("search test test1\n")),
		},
		{
			Description: "dns options",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--dns-opt", "no-tld-query", "--dns-option", "attempts:10", testutil.CommonImage, "cat", "/etc/resolv.conf")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Contains("options no-tld-query attempts:10\n")),
		},
		{
			Description: "dns combined flags",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--dns", "8.8.8.8", "--dns-search", "test", "--dns-option", "attempts:10", testutil.CommonImage, "cat", "/etc/resolv.conf")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
				expect.Contains("nameserver 8.8.8.8\n"),
				expect.Contains("search test\n"),
				expect.Contains("options attempts:10\n"),
			)),
		},
	}

	testCase.Run(t)
}

func TestRunNetworkHostHostname(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		hostname, err := os.Hostname()
		assert.NilError(helpers.T(), err)
		data.Labels().Set("hostname", hostname)
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "hostname command returns host hostname",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--network", "host", testutil.CommonImage, "hostname")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(data.Labels().Get("hostname") + "\n"),
				}
			},
		},
		{
			Description: "HOSTNAME env returns host hostname",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo $HOSTNAME")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: expect.Equals(data.Labels().Get("hostname") + "\n"),
				}
			},
		},
		{
			Description: "hostname override with hostname command",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--network", "host", "--hostname", "override", testutil.CommonImage, "hostname")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("override\n")),
		},
		{
			Description: "hostname override with HOSTNAME env",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--network", "host", "--hostname", "override", testutil.CommonImage, "sh", "-euxc", "echo $HOSTNAME")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("override\n")),
		},
	}

	testCase.Run(t)
}

func TestRunNetworkHost2613(t *testing.T) {
	nerdtest.Setup()

	testCase := &test.Case{
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm", "--add-host", "foo:1.2.3.4", testutil.CommonImage, "getent", "hosts", "foo")
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.Equals("1.2.3.4           foo  foo\n")),
	}

	testCase.Run(t)
}

func TestSharedNetworkSetup(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			data.Labels().Set("container1", data.Identifier("container1"))
			helpers.Ensure("run", "-d", "--name", data.Identifier("container1"),
				testutil.CommonImage, "sleep", "inf")
			nerdtest.EnsureContainerStarted(helpers, data.Identifier("container1"))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier("container1"))
		},
		SubTests: []*test.Case{
			{
				Description: "Test network is shared",
				NoParallel:  true, // The validation involves starting of the main container: container1
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier("container2"))
				},
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("run", "-d", "--name", data.Identifier("container2"), "--network=container:"+data.Labels().Get("container1"), testutil.NginxAlpineImage)
					data.Labels().Set("container2", data.Identifier("container2"))
					nerdtest.EnsureContainerStarted(helpers, data.Identifier("container2"))
				},
				SubTests: []*test.Case{
					{
						NoParallel:  true,
						Description: "Test network is shared",
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("exec", data.Labels().Get("container2"), "wget", "-qO-", "http://127.0.0.1:80")
						},
						Expected: test.Expects(0, nil, expect.Contains(testutil.NginxAlpineIndexHTMLSnippet)),
					},
					{
						NoParallel:  true,
						Description: "Test network is shared after restart",
						Setup: func(data test.Data, helpers test.Helpers) {
							helpers.Ensure("restart", data.Labels().Get("container1"))
							helpers.Ensure("stop", "--time=1", data.Labels().Get("container2"))
							helpers.Ensure("start", data.Labels().Get("container2"))
							nerdtest.EnsureContainerStarted(helpers, data.Labels().Get("container2"))
						},
						Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
							return helpers.Command("exec", data.Labels().Get("container2"), "wget", "-qO-", "http://127.0.0.1:80")

						},
						Expected: test.Expects(0, nil, expect.Contains(testutil.NginxAlpineIndexHTMLSnippet)),
					},
				},
			},
			{
				Description: "Test uts is supported in shared network",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--uts", "host",
						"--network=container:"+data.Labels().Get("container1"),
						testutil.CommonImage)
				},
				Expected: test.Expects(0, nil, nil),
			},
			{
				Description: "Test dns is not supported",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--dns", "0.1.2.3",
						"--network=container:"+data.Labels().Get("container1"),
						testutil.CommonImage)
				},
				// 1 for nerdctl, 125 for docker
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
			},
			{
				Description: "Test dns options is not  supported",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--dns-option", "attempts:5",
						"--network=container:"+data.Labels().Get("container1"),
						testutil.CommonImage, "cat", "/etc/resolv.conf")
				},
				// The Option doesn't throw an error but is never inserted to the resolv.conf
				Expected: test.Expects(0, nil, expect.DoesNotContain("attempts:5")),
			},
			{
				Description: "Test publish is not supported",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--publish", "80:8080",
						"--network=container:"+data.Labels().Get("container1"),
						testutil.AlpineImage)
				},
				// 1 for nerdctl, 125 for docker
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
			},
			{
				Description: "Test hostname is not supported",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm", "--hostname", "test",
						"--network=container:"+data.Labels().Get("container1"),
						testutil.AlpineImage)
				},
				// 1 for nerdctl, 125 for docker
				Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
			},
		},
	}
	testCase.Run(t)
}

func TestSharedNetworkWithNone(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Ensure("run", "-d", "--name", data.Identifier("container1"), "--network", "none",
				testutil.CommonImage, "sleep", "inf")
			nerdtest.EnsureContainerStarted(helpers, data.Identifier("container1"))
		},
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier("container1"))
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				"--network=container:"+data.Identifier("container1"), testutil.CommonImage)
		},
		Expected: test.Expects(expect.ExitCodeSuccess, nil, nil),
	}
	testCase.Run(t)
}

func TestRunContainerInExistingNetNS(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		nerdtest.Rootful,
		require.Not(nerdtest.Docker),
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		netNS, err := netns.NewNetNS(data.Temp().Dir("netns-dir") + "/netns")
		assert.NilError(helpers.T(), err)
		err = netNS.Do(func(netns ns.NetNS) error {
			loopback, err := netlink.LinkByName("lo")
			assert.NilError(helpers.T(), err)
			err = netlink.LinkSetUp(loopback)
			assert.NilError(helpers.T(), err)
			return nil
		})
		assert.NilError(helpers.T(), err)
		data.Labels().Set("netNSPath", netNS.GetPath())

		helpers.Ensure("run", "-d", "--name", data.Identifier(),
			"--network=ns:"+netNS.GetPath(), testutil.NginxAlpineImage)
		nerdtest.EnsureContainerStarted(helpers, data.Identifier())
		time.Sleep(3 * time.Second)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Identifier())
		if netNSPath := data.Labels().Get("netNSPath"); netNSPath != "" {
			loadedNS := netns.LoadNetNS(netNSPath)
			_ = loadedNS.Remove()
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("inspect", "--format", "{{.State.Running}}", data.Identifier())
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(stdout, "true"), "container should be running")

				netNSPath := data.Labels().Get("netNSPath")
				testNetNS, err := ns.GetNS(netNSPath)
				assert.NilError(t, err)
				err = testNetNS.Do(func(ns.NetNS) error {
					curlOut, err := exec.Command("curl", "-s", "http://127.0.0.1:80").Output()
					assert.NilError(t, err)
					assert.Assert(t, strings.Contains(string(curlOut), testutil.NginxAlpineIndexHTMLSnippet))
					return nil
				})
				assert.NilError(t, err)
			},
		}
	}

	testCase.Run(t)
}

func TestRunContainerWithMACAddress(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		helpers.Ensure("network", "create", data.Identifier("bridge"), "--driver", "bridge")
		helpers.Ensure("network", "create", data.Identifier("macvlan"), "--driver", "macvlan")
		helpers.Ensure("network", "create", data.Identifier("ipvlan"), "--driver", "ipvlan")

		data.Labels().Set("networkBridge", data.Identifier("bridge"))
		data.Labels().Set("networkMACvlan", data.Identifier("macvlan"))
		data.Labels().Set("networkIPvlan", data.Identifier("ipvlan"))

		// Get the default MAC address of eth0 on the host network
		cmd := helpers.Command("run", "--rm", "-i", "--network", "host", testutil.CommonImage)
		cmd.Feed(strings.NewReader("ip addr show eth0 | grep ether | awk '{printf $2}'"))
		cmd.Run(&test.Expected{
			Output: func(stdout string, t tig.T) {
				data.Labels().Set("defaultMac", stdout)
			},
		})
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Identifier("bridge"))
		helpers.Anyhow("network", "rm", data.Identifier("macvlan"))
		helpers.Anyhow("network", "rm", data.Identifier("ipvlan"))
	}

	type macTestCase struct {
		networkKey string // label key or literal network name
		isLiteral  bool   // if true, use networkKey as-is; otherwise look up from labels
		wantErr    bool
		expectKey  string // "defaultMac", "passedMac", "", or a literal substring
	}

	macTestCases := []macTestCase{
		{networkKey: "host", isLiteral: true, wantErr: false, expectKey: "defaultMac"},
		{networkKey: "none", isLiteral: true, wantErr: false, expectKey: ""},
		{networkKey: "container:whatever", isLiteral: true, wantErr: true, expectKey: "container"},
		{networkKey: "bridge", isLiteral: true, wantErr: false, expectKey: "passedMac"},
		{networkKey: "networkBridge", isLiteral: false, wantErr: false, expectKey: "passedMac"},
		{networkKey: "networkMACvlan", isLiteral: false, wantErr: false, expectKey: "passedMac"},
		{networkKey: "networkIPvlan", isLiteral: false, wantErr: true, expectKey: "not support"},
	}

	for i, mc := range macTestCases {
		mc := mc
		i := i
		desc := mc.networkKey
		if !mc.isLiteral {
			desc = mc.networkKey + " (custom)"
		}
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: fmt.Sprintf("network %s", desc),
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				macAddress, err := nettestutil.GenerateMACAddress()
				assert.NilError(helpers.T(), err)
				data.Labels().Set(fmt.Sprintf("mac-%d", i), macAddress)

				network := mc.networkKey
				if !mc.isLiteral {
					network = data.Labels().Get(mc.networkKey)
				}

				cmd := helpers.Command("run", "--rm", "-i", "--network", network, "--mac-address", macAddress, testutil.CommonImage)
				cmd.Feed(strings.NewReader("ip addr show eth0 | grep ether | awk '{printf $2}'"))
				return cmd
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				if mc.wantErr {
					return &test.Expected{
						ExitCode: expect.ExitCodeGenericFail,
						Output: func(stdout string, t tig.T) {
							// For error cases, check combined stderr
						},
					}
				}

				expectedStr := ""
				switch mc.expectKey {
				case "defaultMac":
					expectedStr = data.Labels().Get("defaultMac")
				case "passedMac":
					expectedStr = data.Labels().Get(fmt.Sprintf("mac-%d", i))
				case "":
					// no output expected (none network)
				}

				if expectedStr == "" {
					return &test.Expected{
						ExitCode: expect.ExitCodeSuccess,
					}
				}
				return &test.Expected{
					Output: expect.Contains(expectedStr),
				}
			},
		})
	}

	testCase.Run(t)
}

func TestHostsFileMounts(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.Not(nerdtest.RootlessWithDetachNetNS) // etc/hosts is not writable
	testCase.NoParallel = true

	type hostsTestCase struct {
		desc     string
		args     []string
		wantFail bool
	}

	hostsTestCases := []hostsTestCase{
		// /etc/hosts tests
		{
			desc: "write /etc/hosts default network",
			args: []string{"run", "--rm", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/hosts"},
		},
		{
			desc: "write /etc/hosts host network",
			args: []string{"run", "--rm", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/hosts"},
		},
		{
			desc:     "write /etc/hosts host network ro mount fails",
			args:     []string{"run", "--rm", "-v", "/etc/hosts:/etc/hosts:ro", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/hosts"},
			wantFail: true,
		},
		{
			desc: "write /etc/hosts host network rw mount",
			args: []string{"run", "--rm", "-v", "/etc/hosts:/etc/hosts", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/hosts"},
		},
		{
			desc: "restore /etc/hosts host network rw mount",
			args: []string{"run", "--rm", "-v", "/etc/hosts:/etc/hosts", "--network", "host", testutil.CommonImage, "sh", "-euxc", "head -n -1 /etc/hosts > temp && cat temp > /etc/hosts"},
		},
		{
			desc: "write /etc/hosts none network",
			args: []string{"run", "--rm", "--network", "none", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/hosts"},
		},
		// /etc/resolv.conf tests
		{
			desc: "write /etc/resolv.conf default network",
			args: []string{"run", "--rm", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/resolv.conf"},
		},
		{
			desc: "write /etc/resolv.conf host network",
			args: []string{"run", "--rm", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/resolv.conf"},
		},
		{
			desc:     "write /etc/resolv.conf host network ro mount fails",
			args:     []string{"run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf:ro", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/resolv.conf"},
			wantFail: true,
		},
		{
			desc: "write /etc/resolv.conf host network rw mount",
			args: []string{"run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/resolv.conf"},
		},
		{
			desc: "restore /etc/resolv.conf host network rw mount",
			args: []string{"run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf", "--network", "host", testutil.CommonImage, "sh", "-euxc", "head -n -1 /etc/resolv.conf > temp && cat temp > /etc/resolv.conf"},
		},
		{
			desc: "write /etc/resolv.conf host network after restore",
			args: []string{"run", "--rm", "--network", "host", testutil.CommonImage, "sh", "-euxc", "echo >> /etc/resolv.conf"},
		},
	}

	for _, hc := range hostsTestCases {
		hc := hc
		var expected test.Manager
		if hc.wantFail {
			expected = test.Expects(expect.ExitCodeGenericFail, nil, nil)
		} else {
			expected = test.Expects(expect.ExitCodeSuccess, nil, nil)
		}
		testCase.SubTests = append(testCase.SubTests, &test.Case{
			Description: hc.desc,
			NoParallel:  true,
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command(hc.args...)
			},
			Expected: expected,
		})
	}

	testCase.Run(t)
}

func TestRunContainerWithStaticIP6(t *testing.T) {
	testCase := nerdtest.Setup()

	if rootlessutil.IsRootless() && !testutil.RootlessKitIPv6Enabled(t.Context()) {
		t.Skip("Rootless IPv6 requires CONTAINERD_ROOTLESS_ROOTLESSKIT_IPV6=true; see docs/rootless.md")
	}

	networkSubnet := "2001:db8:5::/64"

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("networkName", data.Identifier("ipv6net"))
		data.Labels().Set("networkSubnet", networkSubnet)
		helpers.Ensure("network", "create", data.Identifier("ipv6net"), "--subnet", networkSubnet, "--ipv6")
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("network", "rm", data.Identifier("ipv6net"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "auto-assigned IPv6 within subnet",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--name", data.Identifier("auto"),
					"--network", data.Labels().Get("networkName"),
					testutil.NginxAlpineImage, "ip", "addr", "show", "dev", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						_, subnet, err := net.ParseCIDR(data.Labels().Get("networkSubnet"))
						assert.NilError(t, err)
						ip := nerdtest.FindIPv6(stdout)
						assert.Assert(t, subnet.Contains(ip), "expected subnet %s to include ip %s", subnet, ip)
					},
				}
			},
		},
		{
			Description: "static IPv6 exact match",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--name", data.Identifier("static"),
					"--network", data.Labels().Get("networkName"),
					"--ip6", "2001:db8:5::6",
					testutil.NginxAlpineImage, "ip", "addr", "show", "dev", "eth0")
			},
			Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
				return &test.Expected{
					Output: func(stdout string, t tig.T) {
						_, subnet, err := net.ParseCIDR(data.Labels().Get("networkSubnet"))
						assert.NilError(t, err)
						ip := nerdtest.FindIPv6(stdout)
						assert.Assert(t, subnet.Contains(ip), "expected subnet %s to include ip %s", subnet, ip)
						assert.Equal(t, "2001:db8:5::6", ip.String())
					},
				}
			},
		},
		{
			Description: "static IPv6 outside subnet fails",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("run", "--rm", "--name", data.Identifier("badip6"),
					"--network", data.Labels().Get("networkName"),
					"--ip6", "2001:db8:4::6",
					testutil.NginxAlpineImage, "ip", "addr", "show", "dev", "eth0")
			},
			Expected: test.Expects(expect.ExitCodeGenericFail, nil, nil),
		},
	}

	testCase.Run(t)
}

func TestNoneNetworkHostName(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Cleanup: func(data test.Data, helpers test.Helpers) {
			helpers.Anyhow("rm", "-f", data.Identifier())
		},
		Setup: func(data test.Data, helpers test.Helpers) {
			output := helpers.Capture("run", "-d", "--name", data.Identifier(), "--network", "none", testutil.CommonImage, "sleep", "inf")
			assert.Assert(helpers.T(), len(output) > 12, output)
			data.Labels().Set("hostname", output[:12])
			nerdtest.EnsureContainerStarted(helpers, data.Identifier())
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("exec", data.Identifier(), "cat", "/etc/hostname")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: expect.Equals(data.Labels().Get("hostname") + "\n"),
			}
		},
	}
	testCase.Run(t)
}

func TestHostNetworkHostName(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			helpers.Custom("cat", "/etc/hostname").Run(&test.Expected{
				Output: func(stdout string, t tig.T) {
					data.Labels().Set("hostHostname", stdout)
				},
			})
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				"--network", "host",
				testutil.AlpineImage, "cat", "/etc/hostname")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				Output: expect.Equals(data.Labels().Get("hostHostname")),
			}
		},
	}
	testCase.Run(t)
}

func TestHostNetworkDnsPreserved(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Setup: func(data test.Data, helpers test.Helpers) {
			// In some rootless CI job, slirp provides 10.0.2.3 as DNS server.
			// We cannot simply parse host /etc/resolv.conf here.
			captureNameservers := func(resolvConfPath string) string {
				var nameservers string
				helpers.Command("run", "--rm",
					"-v", resolvConfPath+":/mnt/resolv.conf:ro",
					testutil.AlpineImage,
					"grep", "-E", "^nameserver\\s+", "/mnt/resolv.conf").Run(&test.Expected{
					Output: func(stdout string, t tig.T) {
						nameservers = stdout
					},
				})
				return nameservers
			}
			nameservers := captureNameservers("/etc/resolv.conf")
			// Mirror pkg/resolvconf.Path(): when 127.0.0.53 is the only nameserver, the host
			// runs systemd-resolved, and nerdctl uses the resolv.conf that systemd-resolved
			// generates with the actual upstream nameservers.
			// Docker, on the other hand, keeps the stub for host-network containers.
			if !nerdtest.IsDocker() && strings.TrimSpace(nameservers) == "nameserver 127.0.0.53" {
				nameservers = captureNameservers("/run/systemd/resolve/resolv.conf")
			}
			data.Labels().Set("nameservers", nameservers)
		},
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				"--network", "host",
				testutil.AlpineImage,
				"grep", "-E", "^nameserver\\s+", "/etc/resolv.conf")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			// container with --network=host should have same nameserver as host
			nameservers := data.Labels().Get("nameservers")
			return &test.Expected{
				Output: expect.Equals(nameservers),
			}
		},
	}
	testCase.Run(t)
}

func TestDefaultNetworkDnsNoLocalhost(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				testutil.AlpineImage, "grep", "-E", "^nameserver\\s+(127\\.|::1)", "/etc/resolv.conf")
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1, // no match
			}
		},
	}
	testCase.Run(t)
}

func TestNoneNetworkDnsConfigs(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				"--network", "none",
				"--dns", "0.1.2.3", "--dns-search", "example.com", "--dns-option", "timeout:3", "--dns-option", "attempts:5",
				testutil.CommonImage, "cat", "/etc/resolv.conf")
		},
		Expected: test.Expects(0, nil, expect.Contains(
			"0.1.2.3",
			"example.com",
			"attempts:5",
			"timeout:3",
		)),
	}
	testCase.Run(t)
}

func TestHostNetworkDnsConfigs(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.Not(require.Windows),
		Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
			return helpers.Command("run", "--rm",
				"--network", "host",
				"--dns", "0.1.2.3", "--dns-search", "example.com", "--dns-option", "timeout:3", "--dns-option", "attempts:5",
				testutil.CommonImage, "cat", "/etc/resolv.conf")
		},
		Expected: test.Expects(0, nil, expect.Contains(
			"0.1.2.3",
			"example.com",
			"attempts:5",
			"timeout:3",
		)),
	}
	testCase.Run(t)
}

func TestDNSWithGlobalConfig(t *testing.T) {
	var configContent test.ConfigValue = `debug = false
debug_full = false
dns = ["10.10.10.10", "20.20.20.20"]
dns_opts = ["ndots:2", "timeout:5"]
dns_search = ["example.com", "test.local"]`

	nerdtest.Setup()

	testCase := &test.Case{
		Config: test.WithConfig(nerdtest.NerdctlToml, configContent),
		// NERDCTL_TOML not supported in Docker
		Require: require.Not(nerdtest.Docker),
		SubTests: []*test.Case{
			{
				Description: "Global DNS settings are used when command line options are not provided",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					nerdctlTomlContent := string(helpers.Read(nerdtest.NerdctlToml))
					helpers.T().Log("NERDCTL_TOML file content:\n%s", nerdctlTomlContent)
					cmd := helpers.Command("run", "--rm", testutil.CommonImage, "cat", "/etc/resolv.conf")
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("nameserver 10.10.10.10"),
					expect.Contains("nameserver 20.20.20.20"),
					expect.Contains("search example.com test.local"),
					expect.Contains("options ndots:2 timeout:5"),
				)),
			},
			{
				Description: "Command line DNS options override global config",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					nerdctlTomlContent := string(helpers.Read(nerdtest.NerdctlToml))
					helpers.T().Log("NERDCTL_TOML file content:\n%s", nerdctlTomlContent)
					cmd := helpers.Command("run", "--rm",
						"--dns", "9.9.9.9",
						"--dns-search", "override.com",
						"--dns-opt", "ndots:3",
						testutil.CommonImage, "cat", "/etc/resolv.conf")
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("nameserver 9.9.9.9"),
					expect.Contains("search override.com"),
					expect.Contains("options ndots:3"),
				)),
			},
			{
				Description: "Global DNS settings should also apply when using host network",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					nerdctlTomlContent := string(helpers.Read(nerdtest.NerdctlToml))
					helpers.T().Log("NERDCTL_TOML file content:\n%s", nerdctlTomlContent)
					cmd := helpers.Command("run", "--rm", "--network", "host",
						testutil.CommonImage, "cat", "/etc/resolv.conf")
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("nameserver 10.10.10.10"),
					expect.Contains("nameserver 20.20.20.20"),
					expect.Contains("search example.com test.local"),
					expect.Contains("options ndots:2 timeout:5"),
				)),
			},
			{
				Description: "Global DNS settings should also apply when using none network",
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					nerdctlTomlContent := string(helpers.Read(nerdtest.NerdctlToml))
					helpers.T().Log("NERDCTL_TOML file content:\n%s", nerdctlTomlContent)
					cmd := helpers.Command("run", "--rm", "--network", "none",
						testutil.CommonImage, "cat", "/etc/resolv.conf")
					return cmd
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains("nameserver 10.10.10.10"),
					expect.Contains("nameserver 20.20.20.20"),
					expect.Contains("search example.com test.local"),
					expect.Contains("options ndots:2 timeout:5"),
				)),
			},
		},
	}
	testCase.Run(t)
}

// TestReservePorts tests that a published port appears
// as a listening port on the host.
// See https://github.com/containerd/nerdctl/pull/4526
func TestReservePorts(t *testing.T) {
	nerdtest.Setup()
	testCase := &test.Case{
		Require: require.All(
			require.Not(require.Windows),
			require.Not(nerdtest.RootlessWithoutDetachNetNS), // RootlessKit v1
		),
		NoParallel: true,
		SubTests: []*test.Case{
			{
				Description: "TCP",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("run", "-d", "--name", data.Identifier("nginx"),
						"-p", "60080:80", testutil.NginxAlpineImage)
					nerdtest.EnsureContainerStarted(helpers, data.Identifier("nginx"))
					time.Sleep(3 * time.Second)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier("nginx"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm",
						"--network=host", testutil.CommonImage, "netstat", "-lnt")
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains(":60080"),
				)),
			},
			{
				Description: "UDP",
				Setup: func(data test.Data, helpers test.Helpers) {
					helpers.Ensure("run", "-d", "--name", data.Identifier("coredns"),
						"-p", "60053:53/udp", testutil.CoreDNSImage)
					nerdtest.EnsureContainerStarted(helpers, data.Identifier("coredns"))
					time.Sleep(3 * time.Second)
				},
				Cleanup: func(data test.Data, helpers test.Helpers) {
					helpers.Anyhow("rm", "-f", data.Identifier("coredns"))
				},
				Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
					return helpers.Command("run", "--rm",
						"--network=host", testutil.CommonImage, "netstat", "-lnu")
				},
				Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.All(
					expect.Contains(":60053"),
				)),
			},
		},
	}
	testCase.Run(t)
}

func TestRunExposeOnly(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", data.Identifier())
		helpers.Ensure("run", "-d", "--name", data.Labels().Get("containerName"), "--expose", "8089", testutil.NginxAlpineImage)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "exposed ports are shown in inspect",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("containerName"), "--format", "{{json .Config.ExposedPorts}}")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(stdout, `"80/tcp":{}`), stdout)
				assert.Assert(t, strings.Contains(stdout, `"8089/tcp":{}`), stdout)
			}),
		},
		{
			Description: "expose does not publish ports",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("port", data.Labels().Get("containerName"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, expect.DoesNotContain("8089/tcp ->")),
		},
	}

	testCase.Run(t)
}

func TestRunExposePublishAll(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.Require = require.All(
		require.Not(nerdtest.Rootless), // Automatic port allocation is only supported in rootful mode.
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		data.Labels().Set("containerName", data.Identifier())
		helpers.Ensure("run", "-d", "--name", data.Labels().Get("containerName"), "--expose", "8089", "-P", testutil.NginxAlpineImage)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		helpers.Anyhow("rm", "-f", data.Labels().Get("containerName"))
	}

	testCase.SubTests = []*test.Case{
		{
			Description: "exposed ports are shown in inspect",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("inspect", data.Labels().Get("containerName"), "--format", "{{json .Config.ExposedPorts}}")
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(stdout, `"80/tcp":{}`), stdout)
				assert.Assert(t, strings.Contains(stdout, `"8089/tcp":{}`), stdout)
			}),
		},
		{
			Description: "publish-all publishes image and CLI exposed ports",
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				return helpers.Command("port", data.Labels().Get("containerName"))
			},
			Expected: test.Expects(expect.ExitCodeSuccess, nil, func(stdout string, t tig.T) {
				assert.Assert(t, strings.Contains(stdout, "80/tcp ->"), stdout)
				assert.Assert(t, strings.Contains(stdout, "8089/tcp ->"), stdout)
			}),
		},
	}

	testCase.Run(t)
}
