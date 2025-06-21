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

var (
	AlpineImage         = getImage("alpine")
	BusyboxImage        = getImage("busybox")
	DockerAuthImage     = getImage("docker_auth")
	FluentdImage        = getImage("fluentd")
	GolangImage         = getImage("golang")
	KuboImage           = getImage("kubo")
	MariaDBImage        = getImage("mariadb")
	NginxAlpineImage    = getImage("nginx")
	RegistryImageStable = getImage("registry")
	SystemdImage        = getImage("stargz")
	WordpressImage      = getImage("wordpress")

	CommonImage = AlpineImage

	FedoraESGZImage = getImage("fedora_esgz") // eStargz
	FfmpegSociImage = getImage("ffmpeg_soci") // SOCI
	UbuntuImage     = getImage("ubuntu")      // Large enough for testing soci index creation
)

const (
	// This error string is expected when attempting to connect to a TCP socket
	// for a service which actively refuses the connection.
	// (e.g. attempting to connect using http to an https endpoint).
	// It should be "connection refused" as per the TCP RFC.
	// https://www.rfc-editor.org/rfc/rfc793
	ExpectedConnectionRefusedError = "connection refused"

	NginxAlpineIndexHTMLSnippet = "<title>Welcome to nginx!</title>"
	WordpressIndexHTMLSnippet   = "<title>WordPress &rsaquo; Installation</title>"

	// Source: https://gist.github.com/cpuguy83/fcf3041e5d8fb1bb5c340915aabeebe0
	NonDistBlobImage = "ghcr.io/cpuguy83/non-dist-blob:latest@sha256:8859ffb0bb604463fe19f1e606ceda9f4f8f42e095bf78c42458cf6da7b5c7e7"
	// Foreign layer digest
	NonDistBlobDigest = "sha256:be691b1535726014cdf3b715ff39361b19e121ca34498a9ceea61ad776b9c215"
)
