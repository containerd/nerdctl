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
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
)

func newClient(clicontext *cli.Context) (*containerd.Client, context.Context, context.CancelFunc, error) {
	ctx := context.Background()
	namespace := clicontext.String("namespace")
	ctx = namespaces.WithNamespace(ctx, namespace)
	address := strings.TrimPrefix(clicontext.String("address"), "unix://")
	const dockerContainerdaddress = "/run/docker/containerd/containerd.sock"
	if err := unix.Access(address, unix.R_OK|unix.W_OK); err != nil {
		if unix.Access(dockerContainerdaddress, unix.R_OK|unix.W_OK) == nil {
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
