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
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/compose"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func newComposeStartCommand() *cobra.Command {
	var composeRestartCommand = &cobra.Command{
		Use:                   "start [SERVICE...]",
		Short:                 "Start existing containers for service(s)",
		RunE:                  composeStartAction,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
	}
	return composeRestartCommand
}

func composeStartAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	// TODO(djdongjin): move to `pkg/composer` and rewrite `c.Services + for-loop`
	// with `c.project.WithServices` after refactor (#1639) is done.
	services, err := c.Services(ctx, args...)
	if err != nil {
		return err
	}
	for _, svc := range services {
		svcName := svc.Unparsed.Name
		containers, err := c.Containers(ctx, svcName)
		if err != nil {
			return err
		}
		// return error if no containers and service replica is not zero
		if len(containers) == 0 {
			if len(svc.Containers) == 0 {
				continue
			}
			return fmt.Errorf("service %q has no container to start", svcName)
		}

		if err := startContainers(ctx, client, containers); err != nil {
			return err
		}
	}

	return nil
}

func startContainers(ctx context.Context, client *containerd.Client, containers []containerd.Container) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, c := range containers {
		c := c
		eg.Go(func() error {
			if cStatus, err := containerutil.ContainerStatus(ctx, c); err != nil {
				// NOTE: NotFound doesn't mean that container hasn't started.
				// In docker/CRI-containerd plugin, the task will be deleted
				// when it exits. So, the status will be "created" for this
				// case.
				if !errdefs.IsNotFound(err) {
					return err
				}
			} else if cStatus.Status == containerd.Running {
				return nil
			}

			// in compose, always disable attach
			if err := containerutil.Start(ctx, c, false, client, ""); err != nil {
				return err
			}
			info, err := c.Info(ctx, containerd.WithoutRefreshedMetadata)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintf(os.Stdout, "Container %s started\n", info.Labels[labels.Name])
			return err
		})
	}

	return eg.Wait()
}
