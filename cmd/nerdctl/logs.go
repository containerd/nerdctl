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
		&cli.BoolFlag{
			Name:    "timestamps",
			Aliases: []string{"t"},
			Usage:   "Show timestamps",
		},
		&cli.StringFlag{
			Name:    "tail",
			Aliases: []string{"n"},
			Value:   "all",
			Usage:   "Number of lines to show from the end of the logs",
		},
		&cli.StringFlag{
			Name:  "since",
			Usage: "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)",
		},
		&cli.StringFlag{
			Name:  "until",
			Usage: "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)",
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
			var cmd *exec.Cmd
			//chan for non-follow tail to check the logsEOF
			logsEOFChan := make(chan struct{})
			if clicontext.Bool("follow") && status.Status == containerd.Running {
				waitCh, err := task.Wait(ctx)
				if err != nil {
					return err
				}
				reader, cmd, err = newTailReader(ctx, task, logJSONFilePath, clicontext.Bool("follow"), clicontext.String("tail"))
				if err != nil {
					return err
				}

				go func() {
					<-waitCh
					cmd.Process.Kill()
				}()
			} else {
				if clicontext.String("tail") != "" {
					reader, cmd, err = newTailReader(ctx, task, logJSONFilePath, false, clicontext.String("tail"))
					if err != nil {
						return err
					}
					go func() {
						<-logsEOFChan
						cmd.Process.Kill()
					}()

				} else {
					f, err := os.Open(logJSONFilePath)
					if err != nil {
						return err
					}
					defer f.Close()
					reader = f
					go func() {
						<-logsEOFChan
					}()
				}
			}
			return jsonfile.Decode(clicontext.App.Writer, clicontext.App.ErrWriter, reader, clicontext.Bool("timestamps"), clicontext.String("since"), clicontext.String("until"), logsEOFChan)
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
	bashCompleteContainerNames(clicontext, nil)
}

func newTailReader(ctx context.Context, task containerd.Task, filePath string, follow bool, tail string) (io.Reader, *exec.Cmd, error) {

	var args []string

	if tail != "" {
		args = append(args, "-n")
		if tail == "all" {
			args = append(args, "+0")
		} else {
			args = append(args, tail)
		}
	} else {
		args = append(args, "-n")
		args = append(args, "+0")
	}

	if follow {
		args = append(args, "-f")
	}
	args = append(args, filePath)
	cmd := exec.CommandContext(ctx, "tail", args...)
	cmd.Stderr = os.Stderr
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	return r, cmd, nil
}
