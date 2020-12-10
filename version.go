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
	"fmt"

	"github.com/AkihiroSuda/nerdctl/pkg/version"
	"github.com/urfave/cli/v2"
)

var versionCommand = &cli.Command{
	Name:   "version",
	Usage:  "Show the nerdctl version information",
	Action: versionAction,
}

func versionAction(clicontext *cli.Context) error {
	w := clicontext.App.Writer
	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Version:\t%s\n", version.Version)
	fmt.Fprintf(w, " Git commit:\t%s\n", version.Revision)

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	daemonVersion, err := client.Version(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Server:\n")
	fmt.Fprintf(w, " containerd:\n")
	fmt.Fprintf(w, "  Version:\t%s\n", daemonVersion.Version)
	fmt.Fprintf(w, "  Revision:\t%s\n", daemonVersion.Revision)
	return nil
}
