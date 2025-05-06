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

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/Microsoft/hcsshim"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
)

const (
	// CommonImage.
	//
	// More work needs to be done to support windows containers in test framework
	// for the tests that are run now this image (used in k8s upstream testing) meets the needs
	// use gcr.io/k8s-staging-e2e-test-images/busybox:1.36-1-windows-amd64-ltsc2022 locally on windows 11
	// https://github.com/microsoft/Windows-Containers/issues/179
	BusyboxImage = "gcr.io/k8s-staging-e2e-test-images/busybox:1.36.1-1"
	WindowsNano  = BusyboxImage
	CommonImage  = WindowsNano

	// NOTE(aznashwan): the upstream e2e Nginx test image is actually based on BusyBox.
	NginxAlpineImage            = "registry.k8s.io/e2e-test-images/nginx:1.14-2"
	NginxAlpineIndexHTMLSnippet = "<title>Welcome to nginx!</title>"

	// This error string is expected when attempting to connect to a TCP socket
	// for a service which actively refuses the connection.
	// (e.g. attempting to connect using http to an https endpoint).
	// It should be "connection refused" as per the TCP RFC, but it is the
	// below string constant on Windows.
	// https://www.rfc-editor.org/rfc/rfc793
	ExpectedConnectionRefusedError = "No connection could be made because the target machine actively refused it."
)

var (
	GolangImage = mirrorOf("fixme-test-using-this-image-is-disabled-on-windows")
	AlpineImage = mirrorOf("fixme-test-using-this-image-is-disabled-on-windows")

	hypervContainer     bool
	hypervSupported     bool
	hypervSupportedOnce sync.Once
)

// HyperVSupported is a test helper to check if hyperv is enabled on
// the host. This can be used to skip tests that require virtualization.
func HyperVSupported() bool {
	if s := os.Getenv("NO_HYPERV"); s != "" {
		if b, err := strconv.ParseBool(s); err == nil && b {
			return false
		}
	}
	hypervSupportedOnce.Do(func() {
		// Hyper-V Virtual Machine Management service name
		const hypervServiceName = "vmms"

		m, err := mgr.Connect()
		if err != nil {
			return
		}
		defer m.Disconnect()

		s, err := m.OpenService(hypervServiceName)
		// hyperv service was present
		if err == nil {
			hypervSupported = true
			s.Close()
		}
	})
	return hypervSupported
}

// HyperVContainer is a test helper to check if the container is a
// hyperv type container, lists only running containers
func HyperVContainer(inspect dockercompat.Container) (bool, error) {
	query := hcsshim.ComputeSystemQuery{}
	containersList, err := hcsshim.GetContainers(query)
	if err != nil {
		hypervContainer = false
		return hypervContainer, err
	}

	for _, container := range containersList {
		// have to use IDs, not all containers have name set
		if strings.Contains(container.ID, inspect.ID) {
			if container.SystemType == "VirtualMachine" {
				hypervContainer = true
			}
		}
	}

	return hypervContainer, nil
}
