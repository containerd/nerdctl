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
	"strings"

	securejoin "github.com/cyphar/filepath-securejoin"
)

// Joins the given host path with the given guest path while guaranteeing the provided
// path is scoped within the host path. (i.e. joining with `../..` or symlinks cannot
// lead to a higher level path than the provided hostPath, and are simply ignored)
// Will error out if the guest path is absolute and the drive letter is
// anything other than 'C:/'.
func joinHostPath(hostPath string, guestPath string) (string, error) {
	if len(guestPath) >= 2 && string(guestPath[1]) == ":" {
		if !strings.EqualFold(string(guestPath[0]), "c") {
			// https://github.com/moby/moby/blob/dd3b71d17c614f837c4bba18baed9fa2cb31f1a4/daemon/create_windows.go#L42-L70
			return "", fmt.Errorf("Windows containers currently only support absolute guest paths on drive 'C'. Cannot use %q", guestPath)
		}

		// NOTE: we trim the drive prefix from the guest path completely before calling `SecureJoin`.
		guestPath = guestPath[2:]
	}

	return securejoin.SecureJoin(hostPath, guestPath)
}
