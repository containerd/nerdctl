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
	"os"
	"path/filepath"

	"github.com/AkihiroSuda/nerdctl/pkg/inspecttypes/native"
	"github.com/AkihiroSuda/nerdctl/pkg/mountutil/volumestore"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var volumeInspectCommand = &cli.Command{
	Name:      "inspect",
	Usage:     "Display detailed information on one or more volumes",
	ArgsUsage: "[flags] VOLUME [VOLUME, ...]",
	Action:    volumeInspectAction,
}

func volumeInspectAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return err
	}
	result := make([]native.Volume, clicontext.NArg())
	for i, name := range clicontext.Args().Slice() {
		dataPath := filepath.Join(volStore, name, volumestore.DataDirName)
		if _, err := os.Stat(dataPath); err != nil {
			return err
		}
		r := native.Volume{
			Name:       name,
			Mountpoint: dataPath,
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
