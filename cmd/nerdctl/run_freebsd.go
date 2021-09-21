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
	"context"
	"fmt"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/urfave/cli/v2"
)

func runBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring {
		defaultBashComplete(clicontext)
		return
	}
	if coco.flagTakesValue {
		w := clicontext.App.Writer
		switch coco.flagName {
		case "restart":
			fmt.Fprintln(w, "always")
			fmt.Fprintln(w, "no")
			return
		case "pull":
			fmt.Fprintln(w, "always")
			fmt.Fprintln(w, "missing")
			fmt.Fprintln(w, "never")
			return
		case "net", "network":
			bashCompleteNetworkNames(clicontext, nil)
			return
		}
		defaultBashComplete(clicontext)
		return
	}
	// show image names, unless we have "--rootfs" flag
	if clicontext.Bool("rootfs") {
		defaultBashComplete(clicontext)
		return
	}
	bashCompleteImageNames(clicontext)
}

func WithoutRunMount() func(ctx context.Context, client oci.Client, c *containers.Container, s *oci.Spec) error {
	// not valid on freebsd
	return func(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error { return nil }
}
