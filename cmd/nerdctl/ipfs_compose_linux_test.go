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
	"io"
	"os"
	"strings"
	"testing"

	"github.com/containerd/nerdctl/pkg/testutil"
	"github.com/containerd/nerdctl/pkg/testutil/nettestutil"
	"gotest.tools/v3/assert"
)

func TestIPFSComposeUp(t *testing.T) {
	requiresIPFS(t)
	testutil.DockerIncompatible(t)
	tests := []struct {
		name           string
		snapshotter    string
		pushOptions    []string
		requiresStargz bool
	}{
		{
			name:        "overlayfs",
			snapshotter: "overlayfs",
		},
		{
			name:           "stargz",
			snapshotter:    "stargz",
			pushOptions:    []string{"--estargz"},
			requiresStargz: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := testutil.NewBase(t)
			if tt.requiresStargz {
				requiresStargz(base)
			}
			ipfsImgs := make([]string, 2)
			for i, img := range []string{testutil.WordpressImage, testutil.MariaDBImage} {
				ipfsImgs[i] = pushImageToIPFS(t, base, img, tt.pushOptions...)
			}
			base.Env = append(os.Environ(), "CONTAINERD_SNAPSHOTTER="+tt.snapshotter)
			testComposeUp(t, base, fmt.Sprintf(`
version: '3.1'

services:

  wordpress:
    image: %s
    restart: always
    ports:
      - 8080:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      # workaround for https://github.com/containerd/stargz-snapshotter/issues/444
      - "/run"
      - wordpress:/var/www/html

  db:
    image: %s
    restart: always
    environment:
      MYSQL_DATABASE: exampledb
      MYSQL_USER: exampleuser
      MYSQL_PASSWORD: examplepass
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
    volumes:
      # workaround for https://github.com/containerd/stargz-snapshotter/issues/444
      - "/run"
      - db:/var/lib/mysql

volumes:
  wordpress:
  db:
`, ipfsImgs[0], ipfsImgs[1]))
		})
	}
}

func TestIPFSComposeUpBuild(t *testing.T) {
	requiresIPFS(t)
	testutil.DockerIncompatible(t)
	testutil.RequiresBuild(t)
	base := testutil.NewBase(t)
	ipfsCID := pushImageToIPFS(t, base, testutil.NginxAlpineImage)
	ipfsCIDBase := strings.TrimPrefix(ipfsCID, "ipfs://")

	const dockerComposeYAML = `
services:
  web:
    build: .
    ports:
    - 8080:80
`
	dockerfile := fmt.Sprintf(`FROM localhost:5050/ipfs/%s
COPY index.html /usr/share/nginx/html/index.html
`, ipfsCIDBase)
	indexHTML := t.Name()

	comp := testutil.NewComposeDir(t, dockerComposeYAML)
	defer comp.CleanUp()

	comp.WriteFile("Dockerfile", dockerfile)
	comp.WriteFile("index.html", indexHTML)

	base.ComposeCmd("-f", comp.YAMLFullPath(), "up", "-d", "--build", "--ipfs").AssertOK()
	defer base.Cmd("ipfs", "registry", "down").AssertOK()
	defer base.ComposeCmd("-f", comp.YAMLFullPath(), "down", "-v").Run()

	resp, err := nettestutil.HTTPGet("http://127.0.0.1:8080", 50, false)
	assert.NilError(t, err)
	respBody, err := io.ReadAll(resp.Body)
	assert.NilError(t, err)
	t.Logf("respBody=%q", respBody)
	assert.Assert(t, strings.Contains(string(respBody), indexHTML))
}
