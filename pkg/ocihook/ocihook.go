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

package ocihook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/contrib/apparmor"
	pkgapparmor "github.com/containerd/containerd/pkg/apparmor"
	"github.com/containerd/go-cni"
	gocni "github.com/containerd/go-cni"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type Opts struct {
	Stdin            io.Reader
	Event            string    // "createRuntime" or "postStop"
	FullID           string    // "<NAMESPACE>-<ID>"
	CNI              gocni.CNI // nil for non-CNI mode
	Ports            []gocni.PortMapping
	DefaultAAProfile string // name of the default AppArmor profile
}

func Run(opts *Opts) error {
	if opts == nil || opts.Stdin == nil || opts.Event == "" || opts.FullID == "" {
		return errors.Errorf("invalid Opts")
	}
	var state specs.State
	if err := json.NewDecoder(opts.Stdin).Decode(&state); err != nil {
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
	var handler func(state *specs.State, rootfs string, opts *Opts) error
	switch opts.Event {
	case "createRuntime":
		handler = onCreateRuntime
	case "postStop":
		handler = onPostStop
	default:
		return errors.Errorf("unexpected event %q", opts.Event)
	}
	return handler(&state, rootfs, opts)
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

func getCNINamespaceOpts(opts *Opts) ([]cni.NamespaceOpts, error) {
	return []cni.NamespaceOpts{cni.WithCapabilityPortMap(opts.Ports)}, nil
}

func onCreateRuntime(state *specs.State, rootfs string, opts *Opts) error {
	if opts.DefaultAAProfile != "" && pkgapparmor.HostSupports() {
		// ensure that the default profile is loaded to the host
		if err := apparmor.LoadDefaultProfile(opts.DefaultAAProfile); err != nil {
			logrus.WithError(err).Errorf("failed to load AppArmor profile %q", opts.DefaultAAProfile)
		}
	}
	if opts.CNI != nil {
		cniNSOpts, err := getCNINamespaceOpts(opts)
		if err != nil {
			return err
		}
		nsPath, err := getNetNSPath(state)
		if err != nil {
			return err
		}
		ctx := context.Background()
		if _, err := opts.CNI.Setup(ctx, opts.FullID, nsPath, cniNSOpts...); err != nil {
			return errors.Wrap(err, "failed to call cni.Setup")
		}
	}
	return nil
}

func onPostStop(state *specs.State, rootfs string, opts *Opts) error {
	ctx := context.Background()
	if opts.CNI != nil {
		cniNSOpts, err := getCNINamespaceOpts(opts)
		if err != nil {
			return err
		}
		if err := opts.CNI.Remove(ctx, opts.FullID, "", cniNSOpts...); err != nil {
			logrus.WithError(err).Errorf("failed to call cni.Remove")
			return err
		}
	}
	return nil
}
