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

package main

import (
	"fmt"
	"io"
	"net"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/icmd"
)

// TestRunInternetConnectivity tests Internet connectivity with `apk update`
func TestRunInternetConnectivity(t *testing.T) {
	base := testutil.NewBase(t)
	customNet := testutil.Identifier(t)
	base.Cmd("network", "create", customNet).AssertOK()
	defer base.Cmd("network", "rm", customNet).Run()

	type testCase struct {
		args []string
	}
	customNetID := base.InspectNetwork(customNet).ID
	testCases := []testCase{
		{
			args: []string{"--net", "bridge"},
		},
		{
			args: []string{"--net", customNet},
		},
		{
			args: []string{"--net", customNetID},
		},
		{
			args: []string{"--net", customNetID[:12]},
		},
		{
			args: []string{"--net", "host"},
		},
	}
	for _, tc := range testCases {
		tc := tc // IMPORTANT
		name := "default"
		if len(tc.args) > 0 {
			name = strings.Join(tc.args, "_")
		}
		t.Run(name, func(t *testing.T) {
			args := []string{"run", "--rm"}
			args = append(args, tc.args...)
			args = append(args, testutil.AlpineImage, "apk", "update")
			cmd := base.Cmd(args...)
			cmd.AssertOutContains("OK")
		})
	}
}

// TestRunHostLookup tests hostname lookup
func TestRunHostLookup(t *testing.T) {
	base := testutil.NewBase(t)
	// key: container name, val: network name
	m := map[string]string{
		"c0-in-n0":     "n0",
		"c1-in-n0":     "n0",
		"c2-in-n1":     "n1",
		"c3-in-bridge": "bridge",
	}
	customNets := valuesOfMapStringString(m)
	defer func() {
		for name := range m {
			base.Cmd("rm", "-f", name).Run()
		}
		for netName := range customNets {
			if netName == "bridge" {
				continue
			}
			base.Cmd("network", "rm", netName).Run()
		}
	}()

	// Create networks
	for netName := range customNets {
		if netName == "bridge" {
			continue
		}
		base.Cmd("network", "create", netName).AssertOK()
	}

	// Create nginx containers
	for name, netName := range m {
		cmd := base.Cmd("run",
			"-d",
			"--name", name,
			"--hostname", name+"-foobar",
			"--net", netName,
			testutil.NginxAlpineImage,
		)
		t.Logf("creating host lookup testing container with command: %q", strings.Join(cmd.Command, " "))
		cmd.AssertOK()
	}

	testWget := func(srcContainer, targetHostname string, expected bool) {
		t.Logf("resolving %q in container %q (should success: %+v)", targetHostname, srcContainer, expected)
		cmd := base.Cmd("exec", srcContainer, "wget", "-qO-", "http://"+targetHostname)
		if expected {
			cmd.AssertOutContains(testutil.NginxAlpineIndexHTMLSnippet)
		} else {
			cmd.AssertFail()
		}
	}

	// Tests begin
	testWget("c0-in-n0", "c1-in-n0", true)
	testWget("c0-in-n0", "c1-in-n0.n0", true)
	testWget("c0-in-n0", "c1-in-n0-foobar", true)
	testWget("c0-in-n0", "c1-in-n0-foobar.n0", true)
	testWget("c0-in-n0", "c2-in-n1", false)
	testWget("c0-in-n0", "c2-in-n1.n1", false)
	testWget("c0-in-n0", "c3-in-bridge", false)
	testWget("c1-in-n0", "c0-in-n0", true)
	testWget("c1-in-n0", "c0-in-n0.n0", true)
	testWget("c1-in-n0", "c0-in-n0-foobar", true)
	testWget("c1-in-n0", "c0-in-n0-foobar.n0", true)
}

func TestRunPortWithNoHostPort(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("Auto port assign is not supported rootless mode yet")
	}

	type testCase struct {
		containerPort    string
		runShouldSuccess bool
	}
	testCases := []testCase{
		{
			containerPort:    "80",
			runShouldSuccess: true,
		},
		{
			containerPort:    "80-81",
			runShouldSuccess: true,
		},
		{
			containerPort:    "80-81/tcp",
			runShouldSuccess: true,
		},
	}
	tID := testutil.Identifier(t)
	for i, tc := range testCases {
		i := i
		tc := tc
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			testContainerName := fmt.Sprintf("%s-%d", tID, i)
			base := testutil.NewBase(t)
			defer base.Cmd("rm", "-f", testContainerName).Run()
			pFlag := tc.containerPort
			cmd := base.Cmd("run", "-d",
				"--name", testContainerName,
				"-p", pFlag,
				testutil.NginxAlpineImage)
			var result *icmd.Result
			stdoutContent := ""
			if tc.runShouldSuccess {
				cmd.AssertOK()
			} else {
				cmd.AssertFail()
				return
			}
			portCmd := base.Cmd("port", testContainerName)
			portCmd.Base.T.Helper()
			result = portCmd.Run()
			stdoutContent = result.Stdout() + result.Stderr()
			assert.Assert(cmd.Base.T, result.ExitCode == 0, stdoutContent)
			regexExpression := regexp.MustCompile(`80\/tcp.*?->.*?0.0.0.0:(?P<portNumber>\d{1,5}).*?`)
			match := regexExpression.FindStringSubmatch(stdoutContent)
			paramsMap := make(map[string]string)
			for i, name := range regexExpression.SubexpNames() {
				if i > 0 && i <= len(match) {
					paramsMap[name] = match[i]
				}
			}
			if _, ok := paramsMap["portNumber"]; !ok {
				t.Fail()
				return
			}
			connectURL := fmt.Sprintf("http://%s:%s", "127.0.0.1", paramsMap["portNumber"])
			resp, err := nettestutil.HTTPGet(connectURL, 30, false)
			assert.NilError(t, err)
			respBody, err := io.ReadAll(resp.Body)
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(string(respBody), testutil.NginxAlpineIndexHTMLSnippet))
		})
	}

}

func TestUniqueHostPortAssignement(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("Auto port assign is not supported rootless mode yet")
	}

	type testCase struct {
		containerPort    string
		runShouldSuccess bool
	}

	testCases := []testCase{
		{
			containerPort:    "80",
			runShouldSuccess: true,
		},
		{
			containerPort:    "80-81",
			runShouldSuccess: true,
		},
		{
			containerPort:    "80-81/tcp",
			runShouldSuccess: true,
		},
	}

	tID := testutil.Identifier(t)

	for i, tc := range testCases {
		i := i
		tc := tc
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			testContainerName1 := fmt.Sprintf("%s-%d-1", tID, i)
			testContainerName2 := fmt.Sprintf("%s-%d-2", tID, i)
			base := testutil.NewBase(t)
			defer base.Cmd("rm", "-f", testContainerName1, testContainerName2).Run()

			pFlag := tc.containerPort
			cmd1 := base.Cmd("run", "-d",
				"--name", testContainerName1, "-p",
				pFlag,
				testutil.NginxAlpineImage)

			cmd2 := base.Cmd("run", "-d",
				"--name", testContainerName2, "-p",
				pFlag,
				testutil.NginxAlpineImage)
			var result *icmd.Result
			stdoutContent := ""
			if tc.runShouldSuccess {
				cmd1.AssertOK()
				cmd2.AssertOK()
			} else {
				cmd1.AssertFail()
				cmd2.AssertFail()
				return
			}
			portCmd1 := base.Cmd("port", testContainerName1)
			portCmd2 := base.Cmd("port", testContainerName2)
			portCmd1.Base.T.Helper()
			portCmd2.Base.T.Helper()
			result = portCmd1.Run()
			stdoutContent = result.Stdout() + result.Stderr()
			assert.Assert(t, result.ExitCode == 0, stdoutContent)
			port1, err := extractHostPort(stdoutContent, "80")
			assert.NilError(t, err)
			result = portCmd2.Run()
			stdoutContent = result.Stdout() + result.Stderr()
			assert.Assert(t, result.ExitCode == 0, stdoutContent)
			port2, err := extractHostPort(stdoutContent, "80")
			assert.NilError(t, err)
			assert.Assert(t, port1 != port2, "Host ports are not unique")

			// Make HTTP GET request to container 1
			connectURL1 := fmt.Sprintf("http://%s:%s", "127.0.0.1", port1)
			resp1, err := nettestutil.HTTPGet(connectURL1, 30, false)
			assert.NilError(t, err)
			respBody1, err := io.ReadAll(resp1.Body)
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(string(respBody1), testutil.NginxAlpineIndexHTMLSnippet))

			// Make HTTP GET request to container 2
			connectURL2 := fmt.Sprintf("http://%s:%s", "127.0.0.1", port2)
			resp2, err := nettestutil.HTTPGet(connectURL2, 30, false)
			assert.NilError(t, err)
			respBody2, err := io.ReadAll(resp2.Body)
			assert.NilError(t, err)
			assert.Assert(t, strings.Contains(string(respBody2), testutil.NginxAlpineIndexHTMLSnippet))
		})
	}
}

func TestRunPort(t *testing.T) {
	baseTestRunPort(t, testutil.NginxAlpineImage, testutil.NginxAlpineIndexHTMLSnippet, true)
}

func TestRunWithInvalidPortThenCleanUp(t *testing.T) {
	// docker does not set label restriction to 4096 bytes
	testutil.DockerIncompatible(t)
	t.Parallel()
	base := testutil.NewBase(t)
	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).Run()
	base.Cmd("run", "--rm", "--name", containerName, "-p", "22200-22299:22200-22299", testutil.CommonImage).AssertFail()
	base.Cmd("run", "--rm", "--name", containerName, "-p", "22200-22299:22200-22299", testutil.CommonImage).AssertCombinedOutContains(errdefs.ErrInvalidArgument.Error())
	base.Cmd("run", "--rm", "--name", containerName, testutil.CommonImage).AssertOK()
}

func TestRunContainerWithStaticIP(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("Static IP assignment is not supported rootless mode yet.")
	}
	networkName := "test-network"
	networkSubnet := "172.0.0.0/16"
	base := testutil.NewBase(t)
	cmd := base.Cmd("network", "create", networkName, "--subnet", networkSubnet)
	cmd.AssertOK()
	defer base.Cmd("network", "rm", networkName).Run()
	testCases := []struct {
		ip                string
		shouldSuccess     bool
		useNetwork        bool
		checkTheIPAddress bool
	}{
		{
			ip:                "172.0.0.2",
			shouldSuccess:     true,
			useNetwork:        true,
			checkTheIPAddress: true,
		},
		{
			ip:                "192.0.0.2",
			shouldSuccess:     false,
			useNetwork:        true,
			checkTheIPAddress: false,
		},
		{
			ip:                "10.4.0.2",
			shouldSuccess:     true,
			useNetwork:        false,
			checkTheIPAddress: false,
		},
	}
	tID := testutil.Identifier(t)
	for i, tc := range testCases {
		i := i
		tc := tc
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			testContainerName := fmt.Sprintf("%s-%d", tID, i)
			base := testutil.NewBase(t)
			defer base.Cmd("rm", "-f", testContainerName).Run()
			args := []string{
				"run", "-d", "--name", testContainerName,
			}
			if tc.useNetwork {
				args = append(args, []string{"--network", networkName}...)
			}
			args = append(args, []string{"--ip", tc.ip, testutil.NginxAlpineImage}...)
			cmd := base.Cmd(args...)
			if !tc.shouldSuccess {
				cmd.AssertFail()
				return
			}
			cmd.AssertOK()

			if tc.checkTheIPAddress {
				inspectCmd := base.Cmd("inspect", testContainerName, "--format", "\"{{range .NetworkSettings.Networks}} {{.IPAddress}}{{end}}\"")
				result := inspectCmd.Run()
				stdoutContent := result.Stdout() + result.Stderr()
				assert.Assert(inspectCmd.Base.T, result.ExitCode == 0, stdoutContent)
				if !strings.Contains(stdoutContent, tc.ip) {
					t.Fail()
					return
				}
			}
		})
	}
}

func TestRunDNS(t *testing.T) {
	base := testutil.NewBase(t)

	base.Cmd("run", "--rm", "--dns", "8.8.8.8", testutil.CommonImage,
		"cat", "/etc/resolv.conf").AssertOutContains("nameserver 8.8.8.8\n")
	base.Cmd("run", "--rm", "--dns-search", "test", testutil.CommonImage,
		"cat", "/etc/resolv.conf").AssertOutContains("search test\n")
	base.Cmd("run", "--rm", "--dns-search", "test", "--dns-search", "test1", testutil.CommonImage,
		"cat", "/etc/resolv.conf").AssertOutContains("search test test1\n")
	base.Cmd("run", "--rm", "--dns-opt", "no-tld-query", "--dns-option", "attempts:10", testutil.CommonImage,
		"cat", "/etc/resolv.conf").AssertOutContains("options no-tld-query attempts:10\n")
	cmd := base.Cmd("run", "--rm", "--dns", "8.8.8.8", "--dns-search", "test", "--dns-option", "attempts:10", testutil.CommonImage,
		"cat", "/etc/resolv.conf")
	cmd.AssertOutContains("nameserver 8.8.8.8\n")
	cmd.AssertOutContains("search test\n")
	cmd.AssertOutContains("options attempts:10\n")
}

func TestSharedNetworkStack(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("--network=container:<container name|id> only supports linux now")
	}
	base := testutil.NewBase(t)

	containerName := testutil.Identifier(t)
	defer base.Cmd("rm", "-f", containerName).AssertOK()
	base.Cmd("run", "-d", "--name", containerName,
		testutil.NginxAlpineImage).AssertOK()
	base.EnsureContainerStarted(containerName)

	containerNameJoin := testutil.Identifier(t) + "-network"
	defer base.Cmd("rm", "-f", containerNameJoin).AssertOK()
	base.Cmd("run",
		"-d",
		"--name", containerNameJoin,
		"--network=container:"+containerName,
		testutil.CommonImage,
		"sleep", "infinity").AssertOK()

	base.Cmd("exec", containerNameJoin, "wget", "-qO-", "http://127.0.0.1:80").
		AssertOutContains(testutil.NginxAlpineIndexHTMLSnippet)

	base.Cmd("restart", containerName).AssertOK()
	base.Cmd("stop", "--time=1", containerNameJoin).AssertOK()
	base.Cmd("start", containerNameJoin).AssertOK()
	base.Cmd("exec", containerNameJoin, "wget", "-qO-", "http://127.0.0.1:80").
		AssertOutContains(testutil.NginxAlpineIndexHTMLSnippet)
}

func TestRunContainerWithMACAddress(t *testing.T) {
	base := testutil.NewBase(t)
	tID := testutil.Identifier(t)
	networkBridge := "testNetworkBridge" + tID
	networkMACvlan := "testNetworkMACvlan" + tID
	networkIPvlan := "testNetworkIPvlan" + tID
	base.Cmd("network", "create", networkBridge, "--driver", "bridge").AssertOK()
	base.Cmd("network", "create", networkMACvlan, "--driver", "macvlan").AssertOK()
	base.Cmd("network", "create", networkIPvlan, "--driver", "ipvlan").AssertOK()
	t.Cleanup(func() {
		base.Cmd("network", "rm", networkBridge).Run()
		base.Cmd("network", "rm", networkMACvlan).Run()
		base.Cmd("network", "rm", networkIPvlan).Run()
	})
	tests := []struct {
		Network string
		WantErr bool
		Expect  string
	}{
		{"host", true, "conflicting options"},
		{"none", true, "can't open '/sys/class/net/eth0/address'"},
		{"container:whatever" + tID, true, "conflicting options"},
		{"bridge", false, ""},
		{networkBridge, false, ""},
		{networkMACvlan, false, ""},
		{networkIPvlan, true, "not support"},
	}
	for _, test := range tests {
		macAddress, err := nettestutil.GenerateMACAddress()
		if err != nil {
			t.Errorf("failed to generate MAC address: %s", err)
		}
		if test.Expect == "" && !test.WantErr {
			test.Expect = macAddress
		}
		cmd := base.Cmd("run", "--rm", "--network", test.Network, "--mac-address", macAddress, testutil.CommonImage, "cat", "/sys/class/net/eth0/address")
		if test.WantErr {
			cmd.AssertFail()
			cmd.AssertCombinedOutContains(test.Expect)
		} else {
			cmd.AssertOK()
			cmd.AssertOutContains(test.Expect)
		}
	}
}

func TestHostsFileMounts(t *testing.T) {
	if rootlessutil.IsRootless() {
		if detachedNetNS, _ := rootlessutil.DetachedNetNS(); detachedNetNS != "" {
			t.Skip("/etc/hosts is not writable")
		}
	}
	base := testutil.NewBase(t)

	base.Cmd("run", "--rm", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/hosts").AssertOK()
	base.Cmd("run", "--rm", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/hosts").AssertOK()
	base.Cmd("run", "--rm", "-v", "/etc/hosts:/etc/hosts:ro", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/hosts").AssertFail()
	// add a line into /etc/hosts and remove it.
	base.Cmd("run", "--rm", "-v", "/etc/hosts:/etc/hosts", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/hosts").AssertOK()
	base.Cmd("run", "--rm", "-v", "/etc/hosts:/etc/hosts", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "head -n -1 /etc/hosts > temp && cat temp > /etc/hosts").AssertOK()

	base.Cmd("run", "--rm", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/resolv.conf").AssertOK()
	base.Cmd("run", "--rm", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/resolv.conf").AssertOK()
	base.Cmd("run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf:ro", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/resolv.conf").AssertFail()
	// add a line into /etc/resolv.conf and remove it.
	base.Cmd("run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "echo >> /etc/resolv.conf").AssertOK()
	base.Cmd("run", "--rm", "-v", "/etc/resolv.conf:/etc/resolv.conf", "--network", "host", testutil.CommonImage,
		"sh", "-euxc", "head -n -1 /etc/resolv.conf > temp && cat temp > /etc/resolv.conf").AssertOK()
}

func TestRunContainerWithStaticIP6(t *testing.T) {
	if rootlessutil.IsRootless() {
		t.Skip("Static IP6 assignment is not supported rootless mode yet.")
	}
	networkName := "test-network"
	networkSubnet := "2001:db8:5::/64"
	_, subnet, err := net.ParseCIDR(networkSubnet)
	assert.Assert(t, err == nil)
	base := testutil.NewBaseWithIPv6Compatible(t)
	base.Cmd("network", "create", networkName, "--subnet", networkSubnet, "--ipv6").AssertOK()
	t.Cleanup(func() {
		base.Cmd("network", "rm", networkName).Run()
	})
	testCases := []struct {
		ip                string
		shouldSuccess     bool
		checkTheIPAddress bool
	}{
		{
			ip:                "",
			shouldSuccess:     true,
			checkTheIPAddress: false,
		},
		{
			ip:                "2001:db8:5::6",
			shouldSuccess:     true,
			checkTheIPAddress: true,
		},
		{
			ip:                "2001:db8:4::6",
			shouldSuccess:     false,
			checkTheIPAddress: false,
		},
	}
	tID := testutil.Identifier(t)
	for i, tc := range testCases {
		i := i
		tc := tc
		tcName := fmt.Sprintf("%+v", tc)
		t.Run(tcName, func(t *testing.T) {
			testContainerName := fmt.Sprintf("%s-%d", tID, i)
			base := testutil.NewBaseWithIPv6Compatible(t)
			args := []string{
				"run", "--rm", "--name", testContainerName, "--network", networkName,
			}
			if tc.ip != "" {
				args = append(args, "--ip6", tc.ip)
			}
			args = append(args, []string{testutil.NginxAlpineImage, "ip", "addr", "show", "dev", "eth0"}...)
			cmd := base.Cmd(args...)
			if !tc.shouldSuccess {
				cmd.AssertFail()
				return
			}
			cmd.AssertOutWithFunc(func(stdout string) error {
				ip := findIPv6(stdout)
				if !subnet.Contains(ip) {
					return fmt.Errorf("expected subnet %s include ip %s", subnet, ip)
				}
				if tc.checkTheIPAddress {
					if ip.String() != tc.ip {
						return fmt.Errorf("expected ip %s, got %s", tc.ip, ip)
					}
				}
				return nil
			})
		})
	}
}
