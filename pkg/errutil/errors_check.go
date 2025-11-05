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

import (
	"errors"
	"net/http"
	"strings"
)

// IsErrConnectionRefused return whether err is
// "connect: connection refused"
func IsErrConnectionRefused(err error) bool {
	const errMessage = "connect: connection refused"
	return strings.Contains(err.Error(), errMessage)
}

// IsErrHTTPResponseToHTTPSClient returns whether err is
// "server gave HTTP response to HTTPS client"
func IsErrHTTPResponseToHTTPSClient(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "server gave HTTP response to HTTPS client")
}

// IsErrHTTPSFallbackNeeded returns whether the error indicates that
// HTTPS connection failed and should fallback to plain HTTP.
// This includes:
// - http.ErrSchemeMismatch
// - connection refused errors
// - "server gave HTTP response to HTTPS client" errors
func IsErrHTTPSFallbackNeeded(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, http.ErrSchemeMismatch) ||
		IsErrConnectionRefused(err) ||
		IsErrHTTPResponseToHTTPSClient(err)
}
