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
	"path/filepath"
	"text/tabwriter"

	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/mountutil/volumestore"
	"github.com/urfave/cli/v2"
)

var volumeLsCommand = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List volumes",
	Action:  volumeLsAction,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "Only display volume names",
		},
	},
}

func volumeLsAction(clicontext *cli.Context) error {
	vols, err := getVolumes(clicontext)
	if err != nil {
		return err
	}

	if clicontext.Bool("quiet") {
		for _, v := range vols {
			fmt.Fprintln(clicontext.App.Writer, v.Name)
		}
		return nil
	}

	w := tabwriter.NewWriter(clicontext.App.Writer, 4, 8, 4, ' ', 0)
	fmt.Fprintln(w, "VOLUME NAME\tDIRECTORY")
	for _, v := range vols {
		fmt.Fprintf(w, "%s\t%s\n", v.Name, v.Mountpoint)
	}
	return w.Flush()
}

func getVolumes(clicontext *cli.Context) (map[string]native.Volume, error) {
	volStore, err := getVolumeStore(clicontext)
	if err != nil {
		return nil, err
	}

	dEnts, err := ioutil.ReadDir(volStore)
	if err != nil {
		return nil, err
	}

	res := make(map[string]native.Volume, len(dEnts))

	for _, dEnt := range dEnts {
		name := dEnt.Name()
		dataPath := filepath.Join(volStore, name, volumestore.DataDirName)
		entry := native.Volume{
			Name:       name,
			Mountpoint: dataPath,
		}
		res[name] = entry
	}
	return res, nil
}
