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
	"os"

	"github.com/AkihiroSuda/nerdctl/pkg/idutil"
	"github.com/AkihiroSuda/nerdctl/pkg/logging/jsonfile"
	"github.com/containerd/containerd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var logsCommand = &cli.Command{
	Name:      "logs",
	Usage:     "Fetch the logs of a container. Currently, only containers created with `nerdctl logs -d` are supported.",
	ArgsUsage: "[flags] CONTAINER",
	Action:    logsAction,
}

func logsAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}
	argIDs := clicontext.Args().Slice()

	dataRoot := clicontext.String("data-root")

	ns := clicontext.String("namespace")
	switch ns {
	case "moby", "k8s.io":
		logrus.Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl logs -d`")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var exactID string
	if err := idutil.WalkContainers(ctx, client, argIDs,
		func(ctx context.Context, _ *containerd.Client, shortID, id string) error {
			if exactID != "" {
				return errors.Errorf("ambiguous ID %q", shortID)
			}
			exactID = id
			return nil
		}); err != nil {
		return err
	}
	logJSONFilePath := jsonfile.Path(dataRoot, ns, exactID)
	f, err := os.Open(logJSONFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to open %q, container is not created with `nerdctl logs -d`?", logJSONFilePath)
	}
	defer f.Close()
	return jsonfile.Decode(clicontext.App.Writer, clicontext.App.ErrWriter, f)
}
