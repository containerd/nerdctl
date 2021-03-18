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
	"io"
	"os"
	"os/exec"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var logsCommand = &cli.Command{
	Name:         "logs",
	Usage:        "Fetch the logs of a container. Currently, only containers created with `nerdctl run -d` are supported.",
	ArgsUsage:    "[flags] CONTAINER",
	Action:       logsAction,
	BashComplete: logsBashComplete,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "follow",
			Aliases: []string{"f"},
			Usage:   "Follow log output",
		},
	},
}

func logsAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.Errorf("requires exactly 1 argument")
	}

	dataStore, err := getDataStore(clicontext)
	if err != nil {
		return err
	}

	ns := clicontext.String("namespace")
	switch ns {
	case "moby", "k8s.io":
		logrus.Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl run -d`")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return errors.Errorf("ambiguous ID %q", found.Req)
			}
			logJSONFilePath := jsonfile.Path(dataStore, ns, found.Container.ID())
			if _, err := os.Stat(logJSONFilePath); err != nil {
				return errors.Wrapf(err, "failed to open %q, container is not created with `nerdctl run -d`?", logJSONFilePath)
			}
			task, err := found.Container.Task(ctx, nil)
			if err != nil {
				return err
			}
			status, err := task.Status(ctx)
			if err != nil {
				return err
			}
			var reader io.Reader
			if clicontext.Bool("follow") && status.Status == containerd.Running {
				reader, err = newTailReader(ctx, task, logJSONFilePath)
				if err != nil {
					return err
				}
			} else {
				f, err := os.Open(logJSONFilePath)
				if err != nil {
					return err
				}
				defer f.Close()
				reader = f
			}
			return jsonfile.Decode(clicontext.App.Writer, clicontext.App.ErrWriter, reader)
		},
	}
	req := clicontext.Args().First()
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return errors.Errorf("no such container %s", req)
	}
	return nil
}

func logsBashComplete(clicontext *cli.Context) {
	coco := parseCompletionContext(clicontext)
	if coco.boring || coco.flagTakesValue {
		defaultBashComplete(clicontext)
		return
	}
	// show container names (TODO: only show containers with logs)
	bashCompleteContainerNames(clicontext)
}

func newTailReader(ctx context.Context, task containerd.Task, filePath string) (io.Reader, error) {
	cmd := exec.CommandContext(ctx, "tail", "-f", "-n", "+0", filePath)
	cmd.Stderr = os.Stderr
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	waitCh, err := task.Wait(ctx)
	if err != nil {
		return nil, err
	}
	go func() {
		<-waitCh
		cmd.Process.Kill()
	}()
	return r, nil
}
