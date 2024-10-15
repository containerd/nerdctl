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

package ipfs

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest/registry"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nettestutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/portlock"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestIPFSCompNoBuild(t *testing.T) {
	testCase := nerdtest.Setup()

	const ipfsAddrKey = "ipfsAddrKey"

	var ipfsRegistry *registry.Server

	testCase.Require = test.Require(
		test.Linux,
		test.Not(nerdtest.Docker),
		nerdtest.Registry,
		nerdtest.IPFS,
		// See note below
		// nerdtest.Private,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Start Kubo
		ipfsRegistry = registry.NewKuboRegistry(data, helpers, t, nil, 0, nil)
		ipfsRegistry.Setup(data, helpers)
		data.Set(ipfsAddrKey, fmt.Sprintf("/ip4/%s/tcp/%d", ipfsRegistry.IP, ipfsRegistry.Port))

		// Ensure we have the images
		helpers.Ensure("pull", "--quiet", testutil.WordpressImage)
		helpers.Ensure("pull", "--quiet", testutil.MariaDBImage)
	}

	testCase.SubTests = []*test.Case{
		subtestTestIPFSCompNoB(t, false, false),
		subtestTestIPFSCompNoB(t, true, false),
		subtestTestIPFSCompNoB(t, false, true),
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if ipfsRegistry != nil {
			ipfsRegistry.Cleanup(data, helpers)
		}
		helpers.Anyhow("rmi", "-f", testutil.WordpressImage)
		helpers.Anyhow("rmi", "-f", testutil.MariaDBImage)
	}

	testCase.Run(t)
}

func subtestTestIPFSCompNoB(t *testing.T, stargz bool, byAddr bool) *test.Case {
	t.Helper()

	const ipfsAddrKey = "ipfsAddrKey"
	const mariaImageCIDKey = "mariaImageCIDKey"
	const wordpressImageCIDKey = "wordpressImageCIDKey"
	const composeExtraKey = "composeExtraKey"

	testCase := &test.Case{}

	testCase.Description += "with"

	if !stargz {
		testCase.Description += "-no"
	}
	testCase.Description += "-stargz"

	if !byAddr {
		testCase.Description += "-no"
	}
	testCase.Description += "-byAddr"

	if stargz {
		testCase.Require = nerdtest.Stargz
	}

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		var ipfsCIDWP, ipfsCIDMD string
		if stargz {
			ipfsCIDWP = pushToIPFS(helpers, testutil.WordpressImage, "--estargz")
			ipfsCIDMD = pushToIPFS(helpers, testutil.MariaDBImage, "--estargz")
		} else if byAddr {
			ipfsCIDWP = pushToIPFS(helpers, testutil.WordpressImage, "--ipfs-address="+data.Get(ipfsAddrKey))
			ipfsCIDMD = pushToIPFS(helpers, testutil.MariaDBImage, "--ipfs-address="+data.Get(ipfsAddrKey))
			data.Set(composeExtraKey, "--ipfs-address="+data.Get(ipfsAddrKey))
		} else {
			ipfsCIDWP = pushToIPFS(helpers, testutil.WordpressImage)
			ipfsCIDMD = pushToIPFS(helpers, testutil.MariaDBImage)
		}
		data.Set(wordpressImageCIDKey, ipfsCIDWP)
		data.Set(mariaImageCIDKey, ipfsCIDMD)
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		// NOTE:
		// Removing these images locally forces tests to be sequentials (as IPFS being content addressable,
		// they have the same cid - except for the estargz version obviously)
		// Deliberately electing to not remove them here so that we can parallelize and cut down the running time
		/*
			if data.Get(mariaImageCIDKey) != "" {
				helpers.Anyhow("rmi", "-f", data.Get(mariaImageCIDKey))
				helpers.Anyhow("rmi", "-f", data.Get(wordpressImageCIDKey))
			}
		*/
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		safePort, err := portlock.Acquire(0)
		assert.NilError(helpers.T(), err)
		data.Set("wordpressPort", strconv.Itoa(safePort))
		composeUP(data, helpers, fmt.Sprintf(`
version: '3.1'

services:

  wordpress:
    image: ipfs://%s
    restart: always
    ports:
      - %d:80
    environment:
      WORDPRESS_DB_HOST: db
      WORDPRESS_DB_USER: exampleuser
      WORDPRESS_DB_PASSWORD: examplepass
      WORDPRESS_DB_NAME: exampledb
    volumes:
      - wordpress:/var/www/html

  db:
    image: ipfs://%s
    restart: always
    environment:
      MYSQL_DATABASE: exampledb
      MYSQL_USER: exampleuser
      MYSQL_PASSWORD: examplepass
      MYSQL_RANDOM_ROOT_PASSWORD: '1'
    volumes:
      - db:/var/lib/mysql

volumes:
  wordpress:
  db:
`, data.Get(wordpressImageCIDKey), safePort, data.Get(mariaImageCIDKey)), data.Get(composeExtraKey))
		// FIXME: need to break down composeUP into testable commands instead
		// Right now, this is just a dummy placeholder
		return helpers.Command("info")
	}

	testCase.Expected = test.Expects(0, nil, nil)

	return testCase
}

func TestIPFSCompBuild(t *testing.T) {
	testCase := nerdtest.Setup()

	var ipfsServer test.TestableCommand
	var comp *testutil.ComposeDir

	const mainImageCIDKey = "mainImageCIDKey"
	safePort, err := portlock.Acquire(0)
	assert.NilError(t, err)
	var listenAddr = "localhost:" + strconv.Itoa(safePort)

	testCase.Require = test.Require(
		// Linux only
		test.Linux,
		// Obviously not docker supported
		test.Not(nerdtest.Docker),
		nerdtest.Build,
		nerdtest.IPFS,
	)

	testCase.Setup = func(data test.Data, helpers test.Helpers) {
		// Get alpine
		helpers.Ensure("pull", "--quiet", testutil.NginxAlpineImage)
		// Start a local ipfs backed registry
		// FIXME: this is bad and likely to collide with other tests
		ipfsServer = helpers.Command("ipfs", "registry", "serve", "--listen-registry", listenAddr)
		// Once foregrounded, do not wait for it more than a second
		ipfsServer.Background(1 * time.Second)
		// Apparently necessary to let it start...
		time.Sleep(time.Second)

		// Save nginx to ipfs
		data.Set(mainImageCIDKey, pushToIPFS(helpers, testutil.NginxAlpineImage))

		const dockerComposeYAML = `
services:
  web:
    build: .
    ports:
    - 8081:80
`
		dockerfile := fmt.Sprintf(`FROM %s/ipfs/%s
COPY index.html /usr/share/nginx/html/index.html
`, listenAddr, data.Get(mainImageCIDKey))

		comp = testutil.NewComposeDir(t, dockerComposeYAML)
		comp.WriteFile("Dockerfile", dockerfile)
		comp.WriteFile("index.html", data.Identifier("indexhtml"))
	}

	testCase.Cleanup = func(data test.Data, helpers test.Helpers) {
		if ipfsServer != nil {
			// Close the server once done
			helpers.Anyhow("rmi", "-f", data.Get(mainImageCIDKey))
			ipfsServer.Run(nil)
		}
		if comp != nil {
			helpers.Anyhow("compose", "-f", comp.YAMLFullPath(), "down", "-v")
			comp.CleanUp()
		}
	}

	testCase.Command = func(data test.Data, helpers test.Helpers) test.TestableCommand {
		return helpers.Command("compose", "-f", comp.YAMLFullPath(), "up", "-d", "--build")
	}

	testCase.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
		return &test.Expected{
			Output: func(stdout string, info string, t *testing.T) {
				resp, err := nettestutil.HTTPGet("http://127.0.0.1:8081", 10, false)
				assert.NilError(t, err)
				respBody, err := io.ReadAll(resp.Body)
				assert.NilError(t, err)
				t.Logf("respBody=%q", respBody)
				assert.Assert(t, strings.Contains(string(respBody), data.Identifier("indexhtml")))
			},
		}
	}

	testCase.Run(t)
}

func composeUP(data test.Data, helpers test.Helpers, dockerComposeYAML string, opts string) {
	comp := testutil.NewComposeDir(helpers.T(), dockerComposeYAML)
	// defer comp.CleanUp()

	// Because it might or might not happen, and
	helpers.Anyhow("compose", "-f", comp.YAMLFullPath(), "down", "-v")
	defer helpers.Anyhow("compose", "-f", comp.YAMLFullPath(), "down", "-v")

	projectName := comp.ProjectName()

	args := []string{"compose", "-f", comp.YAMLFullPath()}
	if opts != "" {
		args = append(args, opts)
	}

	helpers.Ensure(append(args, "up", "--quiet-pull", "-d")...)

	helpers.Ensure("volume", "inspect", fmt.Sprintf("%s_db", projectName))
	helpers.Ensure("network", "inspect", fmt.Sprintf("%s_default", projectName))

	checkWordpress := func() error {
		// FIXME: see other notes on using the same port repeatedly
		resp, err := nettestutil.HTTPGet("http://127.0.0.1:"+data.Get("wordpressPort"), 5, false)
		if err != nil {
			return err
		}
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		if !strings.Contains(string(respBody), testutil.WordpressIndexHTMLSnippet) {
			return fmt.Errorf("respBody does not contain %q (%s)", testutil.WordpressIndexHTMLSnippet, string(respBody))
		}
		return nil
	}

	var wordpressWorking bool
	var err error
	// 15 seconds is long enough
	for i := 0; i < 5; i++ {
		err = checkWordpress()
		if err == nil {
			wordpressWorking = true
			break
		}
		time.Sleep(3 * time.Second)
	}

	if !wordpressWorking {
		ccc := helpers.Capture("ps", "-a")
		helpers.T().Log(ccc)
		helpers.T().Error(helpers.Err("logs", projectName+"-wordpress-1"))
		helpers.T().Fatalf("wordpress is not working %v", err)
	}

	helpers.Ensure("compose", "-f", comp.YAMLFullPath(), "down", "-v")
	helpers.Fail("volume", "inspect", fmt.Sprintf("%s_db", projectName))
	helpers.Fail("network", "inspect", fmt.Sprintf("%s_default", projectName))
}
