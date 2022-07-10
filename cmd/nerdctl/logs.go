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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	timetypes "github.com/docker/docker/api/types/time"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newLogsCommand() *cobra.Command {
	var logsCommand = &cobra.Command{
		Use:               "logs [flags] CONTAINER",
		Args:              cobra.ExactArgs(1),
		Short:             "Fetch the logs of a container. Currently, only containers created with `nerdctl run -d` are supported.",
		RunE:              logsAction,
		ValidArgsFunction: logsShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	logsCommand.Flags().BoolP("follow", "f", false, "Follow log output")
	logsCommand.Flags().BoolP("timestamps", "t", false, "Show timestamps")
	logsCommand.Flags().StringP("tail", "n", "all", "Number of lines to show from the end of the logs")
	logsCommand.Flags().String("since", "", "Show logs since timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	logsCommand.Flags().String("until", "", "Show logs before a timestamp (e.g. 2013-01-02T13:23:37Z) or relative (e.g. 42m for 42 minutes)")
	return logsCommand
}

func logsAction(cmd *cobra.Command, args []string) error {
	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	switch ns {
	case "moby", "k8s.io":
		logrus.Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl run -d`")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}
	tail, err := cmd.Flags().GetString("tail")
	if err != nil {
		return err
	}
	timestamps, err := cmd.Flags().GetBool("timestamps")
	if err != nil {
		return err
	}
	since, err := cmd.Flags().GetString("since")
	if err != nil {
		return err
	}
	until, err := cmd.Flags().GetString("until")
	if err != nil {
		return err
	}

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("ambiguous ID %q", found.Req)
			}
			l, err := found.Container.Labels(ctx)
			if err != nil {
				return err
			}
			logConfigFilePath := logging.LogConfigFilePath(dataStore, l[labels.Namespace], found.Container.ID())
			var logConfig logging.LogConfig
			logConfigFileB, err := os.ReadFile(logConfigFilePath)
			if err != nil {
				return err
			}
			if err = json.Unmarshal(logConfigFileB, &logConfig); err != nil {
				return err
			}
			switch logConfig.Driver {
			// TODO: move these logics to pkg/logging
			case "json-file":
				logJSONFilePath := jsonfile.Path(dataStore, ns, found.Container.ID())
				if _, err := os.Stat(logJSONFilePath); err != nil {
					return fmt.Errorf("failed to open %q, container is not created with `nerdctl run -d`?: %w", logJSONFilePath, err)
				}
				task, err := found.Container.Task(ctx, nil)
				if err != nil {
					return err
				}
				status, err := task.Status(ctx)
				if err != nil {
					return err
				}
				if status.Status != containerd.Running {
					follow = false
				}

				reader, execCmd, err := newTailReader(ctx, logJSONFilePath, follow, tail)
				if err != nil {
					return err
				}
				go func() {
					if follow {
						if waitCh, err := task.Wait(ctx); err == nil {
							<-waitCh
						}
						execCmd.Process.Kill()
					}
				}()
				return jsonfile.Decode(os.Stdout, os.Stderr, reader, timestamps, since, until)
			case "journald":
				shortID := found.Container.ID()[:12]
				var journalctlArgs = []string{fmt.Sprintf("SYSLOG_IDENTIFIER=%s", shortID), "--output=cat"}
				if follow {
					journalctlArgs = append(journalctlArgs, "-f")
				}
				if since != "" {
					// using GetTimestamp from moby to keep time format consistency
					ts, err := timetypes.GetTimestamp(since, time.Now())
					if err != nil {
						return fmt.Errorf("invalid value for \"since\": %w", err)
					}
					date, err := prepareJournalCtlDate(ts)
					if err != nil {
						return err
					}
					journalctlArgs = append(journalctlArgs, "--since", date)
				}
				if timestamps {
					logrus.Warnf("unsupported timestamps option for jounrald driver")
				}
				if until != "" {
					// using GetTimestamp from moby to keep time format consistency
					ts, err := timetypes.GetTimestamp(until, time.Now())
					if err != nil {
						return fmt.Errorf("invalid value for \"until\": %w", err)
					}
					date, err := prepareJournalCtlDate(ts)
					if err != nil {
						return err
					}
					journalctlArgs = append(journalctlArgs, "--until", date)
				}
				return logging.FetchLogs(journalctlArgs)
			}
			return nil
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func logsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names (TODO: only show containers with logs)
	return shellCompleteContainerNames(cmd, nil)
}

func newTailReader(ctx context.Context, filePath string, follow bool, tail string) (io.Reader, *exec.Cmd, error) {
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

func prepareJournalCtlDate(t string) (string, error) {
	i, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return "", err
	}
	tm := time.Unix(i, 0)
	s := tm.Format("2006-01-02 15:04:05")
	return s, nil
}
