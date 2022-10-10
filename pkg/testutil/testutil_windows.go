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

func mirrorOf(s string) string {
	// plain mirror, NOT stargz-converted images
	return fmt.Sprintf("ghcr.io/stargz-containers/%s-org", s)
}

const (
	WindowsNano = "gcr.io/k8s-staging-e2e-test-images/busybox:1.29-2"
)

var (
	// TODO: build and set Windows-compatible images for the following
	// (preferably estargz but not mandatory)
	DockerAuthImage = mirrorOf("cesanta/docker_auth:1.7")
	RegistryImage   = mirrorOf("registry:2")

	// CommonImage.
	//
	// More work needs to be done to support windows containers in test framework
	// for the tests that are run now this image (used in k8s upstream testing) meets the needs
	// use gcr.io/k8s-staging-e2e-test-images/busybox:1.29-2-windows-amd64-ltsc2022 locally on windows 11
	// https://github.com/microsoft/Windows-Containers/issues/179
	CommonImage = WindowsNano
)
