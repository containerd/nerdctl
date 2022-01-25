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

var (
	AlpineImage                 = "public.ecr.aws/docker/library/alpine:3.13"
	NginxAlpineImage            = "public.ecr.aws/docker/library/nginx:1.19-alpine"
	NginxAlpineIndexHTMLSnippet = "<title>Welcome to nginx!</title>"
	RegistryImage               = "public.ecr.aws/docker/library/registry:2"
	WordpressImage              = "public.ecr.aws/docker/library/wordpress:5.7"
	WordpressIndexHTMLSnippet   = "<title>WordPress &rsaquo; Installation</title>"
	MariaDBImage                = "public.ecr.aws/docker/library/mariadb:10.5"
	DockerAuthImage             = mirrorOf("cesanta/docker_auth:1.7")

	CommonImage = AlpineImage
)

const (
	FedoraESGZImage = "ghcr.io/stargz-containers/fedora:30-esgz" // eStargz
)
