/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
)

func newClient(clicontext *cli.Context) (*containerd.Client, context.Context, context.CancelFunc, error) {
	ctx := context.Background()
	namespace := clicontext.String("namespace")
	ctx = namespaces.WithNamespace(ctx, namespace)
	address := strings.TrimPrefix(clicontext.String("address"), "unix://")
	const dockerContainerdaddress = "/var/run/docker/containerd/containerd.sock"
	if err := isSocketAccessible(address); err != nil {
		if isSocketAccessible(dockerContainerdaddress) == nil {
			err = errors.Wrapf(err, "cannot access containerd socket %q (hint: try running with `--address %s` to connect to Docker-managed containerd)",
				address, dockerContainerdaddress)
		} else {
			err = errors.Wrapf(err, "cannot access containerd socket %q", address)
		}
		return nil, nil, nil, err
	}
	client, err := containerd.New(address)
	if err != nil {
		return nil, nil, nil, err
	}
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	return client, ctx, cancel, nil
}

func isSocketAccessible(s string) error {
	abs, err := filepath.Abs(s)
	if err != nil {
		return err
	}
	// set AT_EACCESS to allow running nerdctl as a setuid binary
	return unix.Faccessat(-1, abs, unix.R_OK|unix.W_OK, unix.AT_EACCESS)
}

// getDataStore returns a string like "/var/lib/nerdctl/1935db59".
// "1935db9" is from `$(echo -n "/run/containerd/containerd.sock" | sha256sum | cut -c1-8)``
func getDataStore(clicontext *cli.Context) (string, error) {
	dataRoot := clicontext.String("data-root")
	if err := os.MkdirAll(dataRoot, 0700); err != nil {
		return "", err
	}
	addrHash, err := getAddrHash(clicontext.String("address"))
	if err != nil {
		return "", err
	}
	dataStore := filepath.Join(dataRoot, addrHash)
	if err := os.MkdirAll(dataStore, 0700); err != nil {
		return "", err
	}
	return dataStore, nil
}

func getAddrHash(addr string) (string, error) {
	const addrHashLen = 8

	addr = strings.TrimPrefix(addr, "unix://")
	var err error
	addr, err = filepath.EvalSymlinks(addr)
	if err != nil {
		return "", err
	}

	d := digest.SHA256.FromString(addr)
	h := d.Encoded()[0:addrHashLen]
	return h, nil
}
