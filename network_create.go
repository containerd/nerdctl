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
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"

	"github.com/AkihiroSuda/nerdctl/pkg/lockutil"
	"github.com/AkihiroSuda/nerdctl/pkg/netutil"
	"github.com/containerd/containerd/errdefs"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var networkCreateCommand = &cli.Command{
	Name:  "create",
	Usage: "Create a network",
	Description: "NOTE: To isolate CNI bridge, CNI isolation plugin needs to be installed: https://github.com/AkihiroSuda/cni-isolation\n" + "\n" +
		"No support for looking up container IPs by their names yet",
	ArgsUsage: "[flags] NETWORK",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "subnet",
			Usage: "Subnet in CIDR format that represents a network segment, e.g. \"10.5.0.0/16\"",
		},
	},
	Action: networkCreateAction,
}

func networkCreateAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}
	name := clicontext.Args().First()
	if !isValidNetName(name) {
		return errors.Errorf("malformed name %s", name)
	}
	netconfpath := clicontext.String("cni-netconfpath")
	if err := os.MkdirAll(netconfpath, 0755); err != nil {
		return err
	}

	fn := func() error {
		e := &netutil.CNIEnv{
			Path:        clicontext.String("cni-path"),
			NetconfPath: netconfpath,
		}
		ll, err := netutil.ConfigLists(e)
		if err != nil {
			return err
		}
		for _, l := range ll {
			if l.Name == name {
				return errors.Errorf("network with name %s already exists", name)
			}
			// TODO: check CIDR collision
		}
		id, err := netutil.AcquireNextID(ll)
		if err != nil {
			return err
		}

		subnet := clicontext.String("subnet")
		if subnet == "" {
			if id > 255 {
				return errors.Errorf("cannot determine subnet for ID %d, specify --subnet manually", id)
			}
			subnet = fmt.Sprintf("10.4.%d.0/24", id)
		}

		l, err := netutil.GenerateConfigList(e, id, name, subnet)
		if err != nil {
			return err
		}
		filename := filepath.Join(netconfpath, "nerdctl-"+name+".conflist")
		if _, err := os.Stat(filename); err == nil {
			return errdefs.ErrAlreadyExists
		}
		if err := ioutil.WriteFile(filename, l.Bytes, 0644); err != nil {
			return err
		}
		fmt.Fprintf(clicontext.App.Writer, "%d\n", id)
		return nil
	}

	return lockutil.WithDirLock(netconfpath, fn)
}

func isValidNetName(s string) bool {
	ok, err := regexp.MatchString("^[a-zA-Z0-9-]+$", s)
	if err != nil {
		panic(err)
	}
	return ok
}
