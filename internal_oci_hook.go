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
	"io"
	"os"
	"path/filepath"

	"github.com/AkihiroSuda/nerdctl/pkg/ocihook"
	"github.com/AkihiroSuda/nerdctl/pkg/portutil"
	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var internalOCIHookCommand = &cli.Command{
	Name:   "oci-hook",
	Usage:  "OCI hook",
	Action: internalOCIHookAction,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "full-id",
			Usage: "containerd namespace + container ID",
		},
		&cli.StringFlag{
			Name:  "container-state-dir",
			Usage: "e.g. /var/lib/nerdctl/default/deadbeef",
		},
		&cli.StringFlag{
			Name:  "network",
			Usage: "value of `nerdctl run --network`",
		},
		&cli.StringSliceFlag{
			Name:  "p",
			Usage: "value of `nerdctl run -p`",
		},
	},
}

func internalOCIHookAction(clicontext *cli.Context) error {
	event := clicontext.Args().First()
	if event == "" {
		return errors.New("event type needs to be passed")
	}
	containerStateDir := clicontext.String("container-state-dir")
	if containerStateDir == "" {
		return errors.New("missing --container-state-dir")
	}
	if err := os.MkdirAll(containerStateDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create %q", containerStateDir)
	}
	logFilePath := filepath.Join(containerStateDir, "oci-hook."+event+".log")
	logFile, err := os.Create(logFilePath)
	if err != nil {
		return err
	}
	defer logFile.Close()
	logrus.SetOutput(io.MultiWriter(clicontext.App.ErrWriter, logFile))

	var cni gocni.CNI
	switch netstr := clicontext.String("network"); netstr {
	case "none", "host":
	case "bridge":
		// --cni-path is a global flag
		cniPath := clicontext.String("cni-path")
		if cniPath == "" {
			return errors.New("missing --cni-path")
		}
		cni, err = gocni.New(gocni.WithPluginDir([]string{cniPath}), gocni.WithConfListBytes([]byte(defaultBridgeNetwork)))
		if err != nil {
			return err
		}
	default:
		return errors.Errorf("unknown network %q", netstr)
	}

	flagPSlice := clicontext.StringSlice("p")
	ports := make([]gocni.PortMapping, len(flagPSlice))
	for i, p := range flagPSlice {
		pm, err := portutil.ParseFlagP(p)
		if err != nil {
			return err
		}
		ports[i] = *pm
	}

	opts := &ocihook.Opts{
		Stdin:            clicontext.App.Reader,
		Event:            event,
		FullID:           clicontext.String("full-id"),
		CNI:              cni,
		Ports:            ports,
		DefaultAAProfile: defaultAppArmorProfileName,
	}

	return ocihook.Run(opts)
}
