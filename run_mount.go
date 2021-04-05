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
	"path/filepath"

	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/idgen"
	"github.com/containerd/nerdctl/pkg/mountutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func generateMountOpts(clicontext *cli.Context, imageVolumes map[string]struct{}) ([]oci.SpecOpts, []string, error) {
	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return nil, nil, err
	}

	//nolint:golint,prealloc
	var (
		opts        []oci.SpecOpts
		anonVolumes []string
	)
	mounted := make(map[string]struct{})

	if flagVSlice := strutil.DedupeStrSlice(clicontext.StringSlice("v")); len(flagVSlice) > 0 {
		ociMounts := make([]specs.Mount, len(flagVSlice))
		for i, v := range flagVSlice {
			x, err := mountutil.ProcessFlagV(v, volStore)
			if err != nil {
				return nil, nil, err
			}
			ociMounts[i] = x.Mount
			mounted[filepath.Clean(x.Mount.Destination)] = struct{}{}
			if x.AnonymousVolume != "" {
				anonVolumes = append(anonVolumes, x.AnonymousVolume)
			}
		}
		opts = append(opts, oci.WithMounts(ociMounts))
	}

	// imageVolumes are defined in Dockerfile "VOLUME" instruction
	for imgVolRaw := range imageVolumes {
		imgVol := filepath.Clean(imgVolRaw)
		switch imgVol {
		case "/", "/dev", "/sys", "proc":
			return nil, nil, errors.Errorf("invalid VOLUME: %q", imgVolRaw)
		}
		if _, ok := mounted[imgVol]; ok {
			continue
		}
		anonVolName := idgen.GenerateID()
		logrus.Debugf("creating anonymous volume %q, for \"VOLUME %s\"",
			anonVolName, imgVolRaw)
		anonVol, err := volStore.Create(anonVolName)
		if err != nil {
			return nil, nil, err
		}
		m := []specs.Mount{
			{
				Type:        "none",
				Source:      anonVol.Mountpoint,
				Destination: imgVol,
				Options:     []string{"rbind"},
			},
		}
		opts = append(opts, oci.WithMounts(m))
		anonVolumes = append(anonVolumes, anonVolName)
	}

	return opts, anonVolumes, nil
}
