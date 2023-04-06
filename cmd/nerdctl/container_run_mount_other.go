//go:build !windows

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
	securejoin "github.com/cyphar/filepath-securejoin"
)

// Joins the given host path with the given guest path while guaranteeing the provided
// path is scoped within the host path. (i.e. joining with `../..` or symlinks cannot
// lead to a higher level path than the provided hostPath, and are simply ignored)
func joinHostPath(hostPath string, guestPath string) (string, error) {
	return securejoin.SecureJoin(hostPath, guestPath)
}
