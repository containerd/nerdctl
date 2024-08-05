//go:build !windows
// +build !windows

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

/*
Forked from https://github.com/kubernetes/kubernetes/blob/cc60b26dee4768e3c5aa0515bbf4ba1824ad38dc/staging/src/k8s.io/cri-client/pkg/logs/logs_other.go
Copyright The Kubernetes Authors.
Licensed under the Apache License, Version 2.0
*/
package logging

import (
	"os"
)

func openFileShareDelete(path string) (*os.File, error) {
	// Noop. Only relevant for Windows.
	return os.Open(path)
}
