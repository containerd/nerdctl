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

const (
	WindowsNano = "gcr.io/k8s-staging-e2e-test-images/busybox:1.29-2"

	// CommonImage.
	//
	// More work needs to be done to support windows containers in test framework
	// for the tests that are run now this image (used in k8s upstream testing) meets the needs
	// use gcr.io/k8s-staging-e2e-test-images/busybox:1.29-2-windows-amd64-ltsc2022 locally on windows 11
	// https://github.com/microsoft/Windows-Containers/issues/179
	CommonImage = WindowsNano

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

	// Image which includes a 'VOLUME "C:/test_dir"' stanza to be used for Windows testing.
	// https://github.com/containerd/containerd/blob/main/integration/images/volume-copy-up/Dockerfile_windows
	WindowsVolumeMountImage = "ghcr.io/containerd/volume-copy-up:2.1"
)
