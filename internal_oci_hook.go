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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/AkihiroSuda/nerdctl/pkg/portutil"
	"github.com/containerd/containerd/contrib/apparmor"
	pkgapparmor "github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/go-cni"
	gocni "github.com/containerd/go-cni"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"golang.org/x/sys/unix"
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
			Name:  "dns",
			Usage: "value of `nerdctl run --dns`",
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
	if err := internalOCIHookActionPostLogrusInit(clicontext); err != nil {
		logrus.Error(err)
		return err
	}
	return nil
}

func internalOCIHookActionPostLogrusInit(clicontext *cli.Context) error {
	var state specs.State
	if err := json.NewDecoder(clicontext.App.Reader).Decode(&state); err != nil {
		return err
	}
	hs, err := loadSpec(state.Bundle)
	if err != nil {
		return err
	}
	rootfs := hs.Root.Path
	if !filepath.IsAbs(rootfs) {
		rootfs = filepath.Join(state.Bundle, rootfs)
	}
	var handler func(state *specs.State, rootfs string, clicontext *cli.Context) error
	switch event := clicontext.Args().First(); event {
	case "createRuntime":
		handler = onCreateRuntime
	case "postStop":
		handler = onPostStop
	default:
		return errors.Errorf("unexpected event %q", event)
	}
	return handler(&state, rootfs, clicontext)
}

// hookSpec is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/containerd/command/oci-hook.go#L59-L64
type hookSpec struct {
	Root struct {
		Path string `json:"path"`
	} `json:"root"`
}

// loadSpec is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/containerd/command/oci-hook.go#L65-L76
func loadSpec(bundle string) (*hookSpec, error) {
	f, err := os.Open(filepath.Join(bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var s hookSpec
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func newCNI(clicontext *cli.Context) (gocni.CNI, error) {
	cniPath := clicontext.String("cni-path")
	return gocni.New(gocni.WithPluginDir([]string{cniPath}), gocni.WithConfListBytes([]byte(defaultBridgeNetwork)))
}

func getNetNSPath(state *specs.State) (string, error) {
	if state.Pid == 0 {
		return "", errors.New("state.Pid is unset")
	}
	s := fmt.Sprintf("/proc/%d/ns/net", state.Pid)
	if _, err := os.Stat(s); err != nil {
		return "", err
	}
	return s, nil
}

func getCNINamespaceOpts(clicontext *cli.Context) ([]cni.NamespaceOpts, error) {
	var portMappings []cni.PortMapping
	for _, p := range clicontext.StringSlice("p") {
		pm, err := portutil.ParseFlagP(p)
		if err != nil {
			return nil, err
		}
		portMappings = append(portMappings, *pm)
	}
	return []cni.NamespaceOpts{cni.WithCapabilityPortMap(portMappings)}, nil
}

func onCreateRuntime(state *specs.State, rootfs string, clicontext *cli.Context) error {
	if pkgapparmor.HostSupports() {
		// ensure that the default profile is loaded to the host
		// FIXME: export loader functions in pkgapparmor
		defaultAppArmorOpt := apparmor.WithDefaultProfile(defaultAppArmorProfileName)
		dummySpec := &specs.Spec{
			Process: &specs.Process{},
		}
		if err := defaultAppArmorOpt(context.TODO(), nil, nil, dummySpec); err != nil {
			logrus.WithError(err).Errorf("failed to load AppArmor profile %q", defaultAppArmorProfileName)
		}
	}
	ctx := context.Background()
	switch clicontext.String("network") {
	case "none", "host":
		// NOP
	default:
		cniNSOpts, err := getCNINamespaceOpts(clicontext)
		if err != nil {
			return err
		}
		containerStateDir := clicontext.String("container-state-dir")
		stateResolvConfPath := filepath.Join(containerStateDir, "resolv.conf")
		resolvConf, err := os.Create(stateResolvConfPath)
		if err != nil {
			return errors.Wrapf(err, "failed to create %q", stateResolvConfPath)
		}
		if _, err = resolvConf.Write([]byte("search localdomain\n")); err != nil {
			return err
		}
		for _, dns := range clicontext.StringSlice("dns") {
			if _, err = resolvConf.Write([]byte("nameserver " + dns + "\n")); err != nil {
				return err
			}
		}
		if err := resolvConf.Close(); err != nil {
			return err
		}
		containerResolvConfPath := filepath.Join(rootfs, "/etc/resolv.conf")
		if _, err := os.Stat(containerResolvConfPath); err != nil {
			if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0755); err != nil {
				return err
			}
			if err := ioutil.WriteFile(containerResolvConfPath, nil, 0644); err != nil {
				return err
			}
		}
		if err := unix.Mount(stateResolvConfPath, containerResolvConfPath, "none", unix.MS_BIND|unix.MS_PRIVATE, ""); err != nil {
			return errors.Wrapf(err, "failed to mount %q on %q", stateResolvConfPath, containerResolvConfPath)
		}

		cni, err := newCNI(clicontext)
		if err != nil {
			return errors.Wrap(err, "failed to call newCNI")
		}
		nsPath, err := getNetNSPath(state)
		if err != nil {
			return err
		}
		if _, err := cni.Setup(ctx, clicontext.String("full-id"), nsPath, cniNSOpts...); err != nil {
			return errors.Wrap(err, "failed to call cni.Setup")
		}
	}
	return nil
}

func onPostStop(state *specs.State, rootfs string, clicontext *cli.Context) error {
	ctx := context.Background()
	switch clicontext.String("network") {
	case "none", "host":
		// NOP
	default:
		cniNSOpts, err := getCNINamespaceOpts(clicontext)
		if err != nil {
			return err
		}
		cni, err := newCNI(clicontext)
		if err != nil {
			return err
		}
		if err := cni.Remove(ctx, clicontext.String("full-id"), "", cniNSOpts...); err != nil {
			logrus.WithError(err).Errorf("failed to call cni.Remove")
			return err
		}

		containerResolvConfPath := filepath.Join(rootfs, "/etc/resolv.conf")
		_ = unix.Unmount(containerResolvConfPath, unix.MNT_DETACH|unix.MNT_FORCE)

		containerStateDir := clicontext.String("container-state-dir")
		if err := os.RemoveAll(containerStateDir); err != nil {
			logrus.WithError(err).Errorf("failed to remove %q", containerStateDir)
		}
	}
	return nil
}
