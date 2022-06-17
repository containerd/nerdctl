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
	"io"
	"os"
	"os/signal"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func newComposeLogsCommand() *cobra.Command {
	var composeLogsCommand = &cobra.Command{
		Use:           "logs [SERVICE...]",
		Short:         "Show logs of a running container",
		RunE:          composeLogsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	composeLogsCommand.Flags().BoolP("follow", "f", false, "Follow log output.")
	composeLogsCommand.Flags().BoolP("timestamps", "t", false, "Show timestamps")
	composeLogsCommand.Flags().String("tail", "all", "Number of lines to show from the end of the logs")
	composeLogsCommand.Flags().Bool("no-color", false, "Produce monochrome output")
	composeLogsCommand.Flags().Bool("no-log-prefix", false, "Don't print prefix in logs")
	return composeLogsCommand
}

func composeLogsAction(cmd *cobra.Command, args []string) error {
	follow, err := cmd.Flags().GetBool("follow")
	if err != nil {
		return err
	}
	timestamps, err := cmd.Flags().GetBool("timestamps")
	if err != nil {
		return err
	}
	tail, err := cmd.Flags().GetString("tail")
	if err != nil {
		return err
	}
	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}
	noLogPrefix, err := cmd.Flags().GetBool("no-log-prefix")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	lo := logging.LogsOptions{
		Follow:      follow,
		Timestamps:  timestamps,
		Tail:        tail,
		NoColor:     noColor,
		NoLogPrefix: noLogPrefix,
	}

	c, err := getComposer(cmd, client)
	if err != nil {
		return err
	}
	serviceNames, err := c.ServiceNames(args...)
	if err != nil {
		return err
	}
	containers, err := c.Containers(ctx, serviceNames...)
	if err != nil {
		return err
	}

	var (
		runEG errgroup.Group
	)

	logsEOFChan := make(chan string) // value: container name
	interruptChan := make(chan os.Signal, 1)
	logsChan := make(chan map[string]string)
	errs := make(chan error)

	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}

	for _, container := range containers {
		container := container
		runEG.Go(func() error {
			rStdoutPipe, wStdoutPipe := io.Pipe()
			rStderrPipe, wStderrPipe := io.Pipe()
			go WriteContainerLogsToPipe(ctx, client, cmd, dataStore, wStdoutPipe, wStderrPipe, rStdoutPipe, rStderrPipe, errs, lo, container)
			info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}
			name := info.Labels[labels.Name]
			c.FormatLogs(name, logsChan, logsEOFChan, lo, rStdoutPipe, rStderrPipe)
			return nil
		})
	}

	if err := runEG.Wait(); err != nil {
		return err
	}

	signal.Notify(interruptChan, os.Interrupt)
	logsEOFMap := make(map[string]struct{}) // key: container name
selectLoop:
	for {
		// Wait for Ctrl-C, or `nerdctl compose down` in another terminal
		select {
		case err := <-errs:
			return err
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
			logsEOFMap[containerName] = struct{}{}
			if len(logsEOFMap) == len(containers) {
				if lo.Follow {
					logrus.Info("All the containers have exited")
				} else {
					logrus.Debug("All the logs reached EOF")
				}
				close(logsChan)
				break selectLoop
			}
			break selectLoop
		}
	}
	return nil
}
