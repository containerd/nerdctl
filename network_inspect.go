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
	"encoding/json"
	"fmt"

	"github.com/AkihiroSuda/nerdctl/pkg/inspecttypes/native"
	"github.com/AkihiroSuda/nerdctl/pkg/netutil"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var networkInspectCommand = &cli.Command{
	Name:         "inspect",
	Usage:        "Display detailed information on one or more networks",
	ArgsUsage:    "[flags] NETWORK [NETWORK, ...]",
	Description:  "NOTE: The output format is not compatible with Docker.",
	Action:       networkInspectAction,
	BashComplete: networkInspectBashComplete,
}

func networkInspectAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	e := &netutil.CNIEnv{
		Path:        clicontext.String("cni-path"),
		NetconfPath: clicontext.String("cni-netconfpath"),
	}

	ll, err := netutil.ConfigLists(e)
	if err != nil {
		return err
	}

	llMap := make(map[string]*netutil.NetworkConfigList, len(ll))
	for _, l := range ll {
		llMap[l.Name] = l
	}

	result := make([]native.Network, clicontext.NArg())
	for i, name := range clicontext.Args().Slice() {
		if name == "host" || name == "none" {
			return errors.Errorf("pseudo network %q cannot be inspected", name)
		}
		l, ok := llMap[name]
		if !ok {
			return errors.Errorf("no such network: %s", name)
		}
		r := native.Network{
			CNI:       json.RawMessage(l.Bytes),
			NerdctlID: l.NerdctlID,
			File:      l.File,
		}
		result[i] = r
	}
	b, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		return err
	}
	fmt.Fprintln(clicontext.App.Writer, string(b))
	return nil
}

func networkInspectBashComplete(clicontext *cli.Context) {
	if _, ok := isFlagCompletionContext(); ok {
		defaultBashComplete(clicontext)
		return
	}
	// show network names
	bashCompleteNetworkNames(clicontext)

	// For `nerdctl network inspect`, print built-in "bridge" as well
	w := clicontext.App.Writer
	fmt.Fprintln(w, "bridge")
}
