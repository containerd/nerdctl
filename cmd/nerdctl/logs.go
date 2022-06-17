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
	"os/signal"
	"strconv"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/logging/jsonfile"
	"github.com/containerd/nerdctl/pkg/logging/pipetagger"
	timetypes "github.com/docker/docker/api/types/time"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
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

	lo := logging.LogsOptions{
		Follow:     follow,
		Timestamps: timestamps,
		Tail:       tail,
		Since:      since,
		Until:      until,
	}

	logsEOFChan := make(chan string) // value: container name
	logsChan := make(chan map[string]string)
	errs := make(chan error)
	var runEG errgroup.Group
	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("ambiguous ID %q", found.Req)
			}
			info, err := found.Container.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}
			name := info.Labels[labels.Name]
			rStdoutPipe, wStdoutPipe := io.Pipe()
			rStderrPipe, wStderrPipe := io.Pipe()

			stdout, stderr, _ := WriteContainerLogsToPipe(ctx, client, cmd, dataStore, wStdoutPipe, wStderrPipe, rStdoutPipe, rStderrPipe, errs, lo, found.Container)

			stdoutTagger := pipetagger.New(stdout, "", -1, true)
			stderrTagger := pipetagger.New(stderr, "", -1, true)

			for _, v := range []string{"stdout", "stderr"} {
				device := v
				runEG.Go(func() error {
					switch device {
					case "stdout":
						stdoutTagger.Run(logsChan, logsEOFChan, device, name)
					case "stderr":
						stderrTagger.Run(logsChan, logsEOFChan, device, name)
					}
					return nil
				})
			}
			/*go stdoutTagger.Run(logsChan, logsEOFChan, "stdout", name)
			go stderrTagger.Run(logsChan, logsEOFChan, "stderr", name)*/

			return nil
		},
	}
	n, err := walker.Walk(ctx, args[0])
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", args[0])
	}

	go func() error {
		if err := runEG.Wait(); err != nil {
			return err
		}
		logsEOFChan <- args[0]
		return nil
	}()

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt)

selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case _ = <-errs:
			//return nil
		case e := <-logsChan:
			for k, v := range e {
				if k == "stdout" {
					fmt.Fprintf(os.Stdout, v)
				} else if k == "stderr" {
					fmt.Fprintf(os.Stderr, v)
				}
			}
		case sig := <-interruptChan:
			logrus.Debugf("Received signal: %s", sig)
			break selectLoop
		case containerName := <-logsEOFChan:
			if lo.Follow {
				// When `nerdctl logs -f` has exited, we can assume that the container has exited
				logrus.Infof("Container %q exited", containerName)
			} else {
				logrus.Debugf("Logs for container %q reached EOF", containerName)
			}
			break selectLoop
		}
	}

	return nil
}

func WriteContainerLogsToPipe(ctx context.Context, client *containerd.Client, cmd *cobra.Command, dataStore string, wStdoutPipe, wStderrPipe io.WriteCloser, rStdoutPipe, rStderrPipe io.ReadCloser, errs chan error, lo logging.LogsOptions, container containerd.Container) (io.Reader, io.Reader, error) {
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return nil, nil, err
	}
	var stdout, stderr io.Reader
	switch ns {
	case "moby", "k8s.io":
		logrus.Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl run -d`")
	}
	l, err := container.Labels(ctx)
	if err != nil {
		return nil, nil, err
	}
	logConfigFilePath := logging.LogConfigFilePath(dataStore, l[labels.Namespace], container.ID())
	var logConfig logging.LogConfig
	logConfigFileB, err := os.ReadFile(logConfigFilePath)
	if err != nil {
		return nil, nil, err
	}
	if err = json.Unmarshal(logConfigFileB, &logConfig); err != nil {
		return nil, nil, err
	}
	//chan for non-follow tail to check the logsEOF
	logsEOFChan := make(chan struct{})

	switch logConfig.Driver {
	case "json-file":
		logJSONFilePath := jsonfile.Path(dataStore, ns, container.ID())
		if _, err := os.Stat(logJSONFilePath); err != nil {
			return nil, nil, fmt.Errorf("failed to open %q, container is not created with `nerdctl run -d`?: %w", logJSONFilePath, err)
		}
		task, err := container.Task(ctx, nil)
		if err != nil {
			return nil, nil, err
		}
		status, err := task.Status(ctx)
		if err != nil {
			return nil, nil, err
		}
		var reader io.Reader
		var execCmd *exec.Cmd
		rPipeStdout, wPipeStdout := io.Pipe()
		rPipeStderr, wPipeStderr := io.Pipe()

		stdout = rPipeStdout
		stderr = rPipeStderr
		if lo.Follow && status.Status == containerd.Running {
			waitCh, err := task.Wait(ctx)
			if err != nil {
				return nil, nil, err
			}
			reader, execCmd, err = newTailReader(ctx, logJSONFilePath, lo.Follow, lo.Tail)
			if err != nil {
				return nil, nil, err
			}
			go func() {
				<-waitCh
				execCmd.Process.Kill()
				wPipeStdout.Close()
				wPipeStderr.Close()
			}()
		} else {
			if lo.Tail != "" {
				reader, execCmd, err = newTailReader(ctx, logJSONFilePath, false, lo.Tail)
				if err != nil {
					return nil, nil, fmt.Errorf("invalid value for \"since\": %w", err)
				}
				go func() {
					<-logsEOFChan
					execCmd.Process.Kill()
					wPipeStdout.Close()
					wPipeStderr.Close()
				}()

			} else {
				f, err := os.Open(logJSONFilePath)
				if err != nil {
					errs <- err
				}
				defer f.Close()
				reader = f
			}
		}

		if err = jsonfile.Decode(reader, wPipeStdout, wPipeStderr, lo.Timestamps, lo.Since, lo.Until, logsEOFChan); err != nil {
			return nil, nil, fmt.Errorf("invalid value for \"since\": %w", err)
		}
	case "journald":
		shortID := container.ID()[:12]
		var journalctlArgs = []string{fmt.Sprintf("SYSLOG_IDENTIFIER=%s", shortID), "--output=cat"}
		if lo.Follow {
			journalctlArgs = append(journalctlArgs, "-f")
		}
		if lo.Since != "" {
			// using GetTimestamp from moby to keep time format consistency
			ts, err := timetypes.GetTimestamp(lo.Since, time.Now())
			if err != nil {
				return nil, nil, fmt.Errorf("invalid value for \"since\": %w", err)
			}
			date, err := prepareJournalCtlDate(ts)
			if err != nil {
				return nil, nil, err
			}
			journalctlArgs = append(journalctlArgs, "--since", date)
		}
		if lo.Timestamps {
			logrus.Warnf("unsupported timestamps option for jounrald driver")
		}
		if lo.Until != "" {
			// using GetTimestamp from moby to keep time format consistency
			ts, err := timetypes.GetTimestamp(lo.Until, time.Now())
			if err != nil {
				return nil, nil, fmt.Errorf("invalid value for \"until\": %w", err)
			}
			date, err := prepareJournalCtlDate(ts)
			if err != nil {
				return nil, nil, err
			}
			journalctlArgs = append(journalctlArgs, "--until", date)
		}

		if stdout, stderr, err = logging.FetchLogs(journalctlArgs, wStdoutPipe, wStderrPipe); err != nil {
			return nil, nil, err
		}
	}
	return stdout, stderr, err
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

func logsShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names (TODO: only show containers with logs)
	return shellCompleteContainerNames(cmd, nil)
}
