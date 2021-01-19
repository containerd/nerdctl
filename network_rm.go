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
	"os"

	"github.com/AkihiroSuda/nerdctl/pkg/lockutil"
	"github.com/AkihiroSuda/nerdctl/pkg/netutil"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var networkRmCommand = &cli.Command{
	Name:        "rm",
	Aliases:     []string{"remove"},
	Usage:       "Remove one or more networks",
	ArgsUsage:   "[flags] NETWORK [NETWORK, ...]",
	Description: "NOTE: network in use is deleted without caution",
	Action:      networkRmAction,
}

func networkRmAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}
	e := &netutil.CNIEnv{
		Path:        clicontext.String("cni-path"),
		NetconfPath: clicontext.String("cni-netconfpath"),
	}
	netconfpath := clicontext.String("cni-netconfpath")
	fn := func() error {
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return err
		}

		llMap := make(map[string]*netutil.NetworkConfigList, len(ll))
		for _, l := range ll {
			llMap[l.Name] = l
		}

		for _, name := range clicontext.Args().Slice() {
			if name == "host" || name == "none" {
				return errors.Errorf("pseudo network %q cannot be removed", name)
			}
			l, ok := llMap[name]
			if !ok {
				return errors.Errorf("no such network: %s", name)
			}
			if l.NerdctlID == nil {
				return errors.Errorf("%s is managed outside nerdctl and cannot be removed", name)
			}
			if l.File == "" {
				return errors.Errorf("%s is a pre-defined network and cannot be removed", name)
			}
			if err := os.RemoveAll(l.File); err != nil {
				return err
			}
			fmt.Fprintln(clicontext.App.Writer, name)
		}
		return nil
	}
	return lockutil.WithDirLock(netconfpath, fn)
}
