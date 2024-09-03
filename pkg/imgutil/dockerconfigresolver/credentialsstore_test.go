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

package dockerconfigresolver

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gotest.tools/v3/assert"
)

func createTempDir(t *testing.T, mode os.FileMode) string {
	tmpDir, err := os.MkdirTemp(t.TempDir(), "docker-config")
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chmod(tmpDir, mode)
	if err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

func TestBrokenCredentialsStore(t *testing.T) {
	if runtime.GOOS == "freebsd" {
		// It is unclear why these tests are failing on FreeBSD, and if it is a problem with Vagrant or differences
		// with FreeBSD
		// Anyhow, this test is about extreme cases & conditions (filesystem errors wrt credentials loading).
		t.Skip("skipping broken credential store tests for freebsd")
	}

	testCases := []struct {
		description string
		setup       func() string
		errorNew    error
		errorRead   error
		errorWrite  error
	}{
		{
			description: "Pointing DOCKER_CONFIG at a non-existent directory inside an unreadable directory will prevent instantiation",
			setup: func() string {
				tmpDir := createTempDir(t, 0000)
				return filepath.Join(tmpDir, "doesnotexistcantcreate")
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a non-existent directory inside a read-only directory will prevent saving credentials",
			setup: func() string {
				tmpDir := createTempDir(t, 0500)
				return filepath.Join(tmpDir, "doesnotexistcantcreate")
			},
			errorWrite: ErrUnableToStore,
		},
		{
			description: "Pointing DOCKER_CONFIG at an unreadable directory will prevent instantiation",
			setup: func() string {
				return createTempDir(t, 0000)
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a read-only directory will prevent saving credentials",
			setup: func() string {
				return createTempDir(t, 0500)
			},
			errorWrite: ErrUnableToStore,
		},
		{
			description: "Pointing DOCKER_CONFIG at a directory containing am unparsable `config.json` will prevent instantiation",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("porked"), 0600)
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a file instead of a directory will prevent instantiation",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				fd, err := os.OpenFile(filepath.Join(tmpDir, "isafile"), os.O_CREATE, 0600)
				if err != nil {
					t.Fatal(err)
				}
				err = fd.Close()
				if err != nil {
					t.Fatal(err)
				}
				return filepath.Join(tmpDir, "isafile")
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a directory containing a `config.json` directory will prevent instantiation",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				err := os.Mkdir(filepath.Join(tmpDir, "config.json"), 0600)
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a directory containing a `config.json` dangling symlink will still work",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				err := os.Symlink("doesnotexist", filepath.Join(tmpDir, "config.json"))
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
		},
		{
			description: "Pointing DOCKER_CONFIG at a directory containing an unreadable, valid `config.json` file will prevent instantiation",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{}"), 0600)
				if err != nil {
					t.Fatal(err)
				}
				err = os.Chmod(filepath.Join(tmpDir, "config.json"), 0000)
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			errorNew: ErrUnableToInstantiate,
		},
		{
			description: "Pointing DOCKER_CONFIG at a directory containing a read-only, valid `config.json` file will NOT prevent saving credentials",
			setup: func() string {
				tmpDir := createTempDir(t, 0700)
				err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte("{}"), 0600)
				if err != nil {
					t.Fatal(err)
				}
				err = os.Chmod(filepath.Join(tmpDir, "config.json"), 0400)
				if err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
		},
	}

	t.Run("Broken Docker Config testing", func(t *testing.T) {
		registryURL, err := Parse("registry")
		if err != nil {
			t.Fatal(err)
		}

		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				directory := tc.setup()
				cs, err := NewCredentialsStore(directory)
				assert.ErrorIs(t, err, tc.errorNew)
				if err != nil {
					return
				}

				var af *Credentials
				af, err = cs.Retrieve(registryURL, true)
				assert.ErrorIs(t, err, tc.errorRead)

				err = cs.Store(registryURL, af)
				assert.ErrorIs(t, err, tc.errorWrite)
			})
		}
	})
}

func writeContent(t *testing.T, content string) string {
	t.Helper()
	tmpDir := createTempDir(t, 0700)
	err := os.WriteFile(filepath.Join(tmpDir, "config.json"), []byte(content), 0600)
	if err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

func TestWorkingCredentialsStore(t *testing.T) {
	testCases := []struct {
		description string
		setup       func() string
		username    string
		password    string
	}{
		{
			description: "Reading credentials from `auth` using canonical identifier",
			username:    "username",
			password:    "password",
			setup: func() string {
				content := fmt.Sprintf(`{
				"auths": {
					"registry.example:443": {
						"auth": %q
					}
				}
			}`, base64.StdEncoding.EncodeToString([]byte("username:password")))
				return writeContent(t, content)
			},
		},
		{
			description: "Reading from legacy / alternative identifiers: registry.example",
			username:    "username",
			setup: func() string {
				content := `{
				"auths": {
					"registry.example": {
						"username": "username"
					}
				}
			}`
				return writeContent(t, content)
			},
		},
		{
			description: "Reading from legacy / alternative identifiers: http://registry.example",
			username:    "username",
			setup: func() string {
				content := `{
				"auths": {
					"http://registry.example": {
						"username": "username"
					}
				}
			}`
				return writeContent(t, content)
			},
		},
		{
			description: "Reading from legacy / alternative identifiers: https://registry.example",
			username:    "username",
			setup: func() string {
				content := `{
				"auths": {
					"https://registry.example": {
						"username": "username"
					}
				}
			}`
				return writeContent(t, content)
			},
		},
		{
			description: "Reading from legacy / alternative identifiers: http://registry.example:443",
			username:    "username",
			setup: func() string {
				content := `{
				"auths": {
					"http://registry.example:443": {
						"username": "username"
					}
				}
			}`
				return writeContent(t, content)
			},
		},
		{
			description: "Reading from legacy / alternative identifiers: https://registry.example:443",
			username:    "username",
			setup: func() string {
				content := `{
				"auths": {
					"https://registry.example:443": {
						"username": "username"
					}
				}
			}`
				return writeContent(t, content)
			},
		},
		{
			description: "Canonical form is preferred over legacy forms",
			username:    "pick",
			setup: func() string {
				content := `{
	"auths": {
		"http://registry.example:443": {
			"username": "ignore"
		},
		"https://registry.example:443": {
			"username": "ignore"
		},
		"registry.example": {
			"username": "ignore"
		},
		"registry.example:443": {
			"serveraddress": "bla",
			"username": "pick"
		},
		"http://registry.example": {
			"username": "ignore"
		},
		"https://registry.example": {
			"username": "ignore"
		}
	}
}`
				return writeContent(t, content)
			},
		},
	}

	t.Run("Working credentials store", func(t *testing.T) {

		for _, tc := range testCases {
			t.Run(tc.description, func(t *testing.T) {
				registryURL, err := Parse("registry.example")
				if err != nil {
					t.Fatal(err)
				}
				cs, err := NewCredentialsStore(tc.setup())
				if err != nil {
					t.Fatal(err)
				}

				var af *Credentials
				af, err = cs.Retrieve(registryURL, true)
				assert.ErrorIs(t, err, nil)
				assert.Equal(t, af.Username, tc.username)
				assert.Equal(t, af.ServerAddress, "registry.example:443")
				assert.Equal(t, af.Password, tc.password)
			})
		}
	})

	t.Run("Namespaced host", func(t *testing.T) {
		server := "host.example/path?ns=namespace.example"
		registryURL, err := Parse(server)
		if err != nil {
			t.Fatal(err)
		}

		content := `{
				"auths": {
					"nerdctl-experimental://namespace.example:443/host/host.example:443/path": {
						"username": "username"
					}
				}
			}`
		dir := writeContent(t, content)
		cs, err := NewCredentialsStore(dir)
		if err != nil {
			t.Fatal(err)
		}

		var af *Credentials
		af, err = cs.Retrieve(registryURL, true)
		assert.ErrorIs(t, err, nil)
		assert.Equal(t, af.Username, "username")
		assert.Equal(t, af.ServerAddress, "host.example:443/path?ns=namespace.example")

	})
}
