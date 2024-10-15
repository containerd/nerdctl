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

import "fmt"

const (
	CommonImage = "docker.io/knast/freebsd:13-STABLE"

	// This error string is expected when attempting to connect to a TCP socket
	// for a service which actively refuses the connection.
	// (e.g. attempting to connect using http to an https endpoint).
	// It should be "connection refused" as per the TCP RFC.
	// https://www.rfc-editor.org/rfc/rfc793
	ExpectedConnectionRefusedError = "connection refused"
)

var (
	AlpineImage      = mirrorOf("alpine:3.13")
	NginxAlpineImage = mirrorOf("nginx:1.19-alpine")
	GolangImage      = mirrorOf("golang:1.18")
)

func mirrorOf(s string) string {
	// plain mirror, NOT stargz-converted images
	return fmt.Sprintf("ghcr.io/stargz-containers/%s-org", s)
}
