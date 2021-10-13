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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/identifiers"
	"github.com/containerd/nerdctl/pkg/lockutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

func newNetworkCreateCommand() *cobra.Command {
	var networkCreateCommand = &cobra.Command{
		Use:           "create [flags] NETWORK",
		Short:         "Create a network",
		Long:          "NOTE: To isolate CNI bridge, CNI isolation plugin needs to be installed: https://github.com/AkihiroSuda/cni-isolation",
		Args:          cobra.ExactArgs(1),
		RunE:          networkCreateAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	networkCreateCommand.Flags().String("subnet", "", `Subnet in CIDR format that represents a network segment, e.g. "10.5.0.0/16"`)
	networkCreateCommand.Flags().StringSlice("label", nil, "Set metadata for a network")
	return networkCreateCommand
}

func networkCreateAction(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}
	name := args[0]
	if err := identifiers.Validate(name); err != nil {
		return errors.Wrapf(err, "malformed name %s", name)
	}
	cniPath, err := cmd.Flags().GetString("cni-path")
	if err != nil {
		return err
	}
	cniNetconfpath, err := cmd.Flags().GetString("cni-netconfpath")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cniNetconfpath, 0755); err != nil {
		return err
	}
	subnet, err := cmd.Flags().GetString("subnet")
	if err != nil {
		return err
	}
	labels, err := cmd.Flags().GetStringSlice("label")
	if err != nil {
		return err
	}

	fn := func() error {
		e := &netutil.CNIEnv{
			Path:        cniPath,
			NetconfPath: cniNetconfpath,
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

		if subnet == "" {
			if id > 255 {
				return errors.Errorf("cannot determine subnet for ID %d, specify --subnet manually", id)
			}
			subnet = fmt.Sprintf("10.4.%d.0/24", id)
		}

		labels := strutil.DedupeStrSlice(labels)
		l, err := netutil.GenerateConfigList(e, labels, id, name, subnet)
		if err != nil {
			return err
		}
		filename := filepath.Join(cniNetconfpath, "nerdctl-"+name+".conflist")
		if _, err := os.Stat(filename); err == nil {
			return errdefs.ErrAlreadyExists
		}
		if err := ioutil.WriteFile(filename, l.Bytes, 0644); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%d\n", id)
		return nil
	}

	return lockutil.WithDirLock(cniNetconfpath, fn)
}
