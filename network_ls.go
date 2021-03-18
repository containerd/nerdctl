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
	"strconv"
	"text/tabwriter"

	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/urfave/cli/v2"
)

var networkLsCommand = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List networks",
	Action:  networkLsAction,
}

func networkLsAction(clicontext *cli.Context) error {
	e := &netutil.CNIEnv{
		Path:        clicontext.String("cni-path"),
		NetconfPath: clicontext.String("cni-netconfpath"),
	}
	ll, err := netutil.ConfigLists(e)
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "NETWORK ID\tNAME\tFILE")
	for _, l := range ll {
		var idStr string
		if l.NerdctlID != nil {
			idStr = strconv.Itoa(*l.NerdctlID)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", idStr, l.Name, l.File)
	}
	// pseudo networks
	fmt.Fprintf(w, "\thost\t\n")
	fmt.Fprintf(w, "\tnone\t\n")
	return w.Flush()
}
