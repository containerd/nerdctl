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

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/netutil"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var composeCommand = &cli.Command{
	Name:  "compose",
	Usage: "Compose",
	Subcommands: []*cli.Command{
		composeUpCommand,
		composeLogsCommand,
		composeBuildCommand,
		composeDownCommand,
	},

	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "file",
			Aliases: []string{"f"},
			Usage:   "Specify an alternate compose file",
		},
		&cli.StringFlag{
			Name:    "project-name",
			Aliases: []string{"p"},
			Usage:   "Specify an alternate project name",
		},
	},
}

func getComposer(clicontext *cli.Context, client *containerd.Client) (*composer.Composer, error) {
	nerdctlCmd, nerdctlArgs := globalFlags(clicontext)
	o := composer.Options{
		File:           clicontext.String("file"),
		Project:        clicontext.String("project-name"),
		NerdctlCmd:     nerdctlCmd,
		NerdctlArgs:    nerdctlArgs,
		DebugPrintFull: clicontext.Bool("debug-full"),
	}

	cniEnv := &netutil.CNIEnv{
		Path:        clicontext.String("cni-path"),
		NetconfPath: clicontext.String("cni-netconfpath"),
	}
	configLists, err := netutil.ConfigLists(cniEnv)
	if err != nil {
		return nil, err
	}

	o.NetworkExists = func(netName string) (bool, error) {
		for _, f := range configLists {
			if f.Name == netName {
				return true, nil
			}
		}
		return false, nil
	}

	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return nil, err
	}

	o.VolumeExists = func(volName string) (bool, error) {
		if _, volGetErr := volStore.Get(volName); volGetErr == nil {
			return true, nil
		} else if errors.Is(volGetErr, errdefs.ErrNotFound) {
			return false, nil
		} else {
			return false, volGetErr
		}
	}

	o.ImageExists = func(ctx context.Context, rawRef string) (bool, error) {
		named, err := refdocker.ParseDockerRef(rawRef)
		if err != nil {
			return false, err
		}
		ref := named.String()
		if _, err := client.ImageService().Get(ctx, ref); err != nil {
			if errors.Is(err, errdefs.ErrNotFound) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	}

	insecure := clicontext.Bool("insecure-registry")
	o.EnsureImage = func(ctx context.Context, imageName, pullMode string) error {
		_, imgErr := imgutil.EnsureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), imageName,
			pullMode, insecure)
		return imgErr
	}

	return composer.New(o)
}
