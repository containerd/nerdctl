//go:build freebsd || linux
// +build freebsd linux

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

	"github.com/containerd/nerdctl/pkg/infoutil"

	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func bashCompleteNamespaceNames(clicontext *cli.Context) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()
	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		logrus.Warn(err)
		return
	}
	for _, ns := range nsList {
		fmt.Fprintln(clicontext.App.Writer, ns)
	}
}

func bashCompleteSnapshotterNames(clicontext *cli.Context) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()
	snapshotterPlugins, err := infoutil.GetSnapshotterNames(ctx, client.IntrospectionService())
	if err != nil {
		return
	}
	for _, name := range snapshotterPlugins {
		fmt.Fprintln(clicontext.App.Writer, name)
	}
}
