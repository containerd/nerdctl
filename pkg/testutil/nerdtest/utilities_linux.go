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

package nerdtest

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
)

const SignalCaught = "received"

var SigQuit os.Signal = syscall.SIGQUIT
var SigUsr1 os.Signal = syscall.SIGUSR1

func RunSigProxyContainer(signal os.Signal, exitOnSignal bool, args []string, data test.Data, helpers test.Helpers) test.TestableCommand {
	sig := strconv.Itoa(int(signal.(syscall.Signal)))
	ready := "trap ready"
	script := `#!/bin/sh
	set -eu

	sig_msg () {
		printf "` + SignalCaught + `\n"
		[ "` + strconv.FormatBool(exitOnSignal) + `" != true ] || exit 0
	}

	trap sig_msg ` + sig + `
	printf "` + ready + `\n"
	while true; do
		printf "waiting...\n"
		sleep 0.5
	done
`

	args = append(args, "--name", data.Identifier(), testutil.CommonImage, "sh", "-c", script)
	args = append([]string{"run"}, args...)

	cmd := helpers.Command(args...)
	// NOTE: because of a test like TestStopWithStopSignal, we need to wait enough for nerdctl to terminate the container
	// It looks like EL8 could be particularly slow (https://github.com/containerd/nerdctl/issues/4068)
	// Note that in normal circumstances, 10 seconds is plenty enough.
	cmd.WithTimeout(40 * time.Second)
	cmd.Background()
	EnsureContainerStarted(helpers, data.Identifier())

	for {
		out := helpers.Capture("logs", data.Identifier())
		if strings.Contains(out, ready) {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return cmd
}

// StartHTTPServer starts an HTTP server bound to 0.0.0.0 and returns a URL reachable
// from processes that cannot access 127.0.0.1 due to namespace isolation.
// It also returns a cleanup function that stops the server.
func StartHTTPServer(handler http.Handler) (url string, stop func(), err error) {
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return "", nil, err
	}
	srv := &httptest.Server{Config: &http.Server{Handler: handler}}
	srv.Listener = l
	srv.Start()
	hostIP, herr := nettestutil.NonLoopbackIPv4()
	if herr != nil {
		srv.Close()
		return "", nil, herr
	}
	_, port, perr := net.SplitHostPort(l.Addr().String())
	if perr != nil {
		srv.Close()
		return "", nil, perr
	}
	return "http://" + hostIP.String() + ":" + port, func() { srv.Close() }, nil
}
