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

package errutil

import "strings"

// IsErrConnectionRefused return whether err is
// "connect: connection refused"
func IsErrConnectionRefused(err error) bool {
	const errMessage = "connect: connection refused"
	return strings.Contains(err.Error(), errMessage)
}

// IsErrHTTPResponseToHTTPSClient returns whether err is
// "http: server gave HTTP response to HTTPS client"
func IsErrHTTPResponseToHTTPSClient(err error) bool {
	const errMessage = "server gave HTTP response to HTTPS client"
	return strings.Contains(err.Error(), errMessage)
}

// IsErrTLSHandshakeFailure returns whether err is a TLS handshake or certificate verification error
func IsErrTLSHandshakeFailure(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "tls:") ||
		strings.Contains(errStr, "x509:") ||
		strings.Contains(errStr, "certificate")
}
