//go:build freebsd || linux
// +build freebsd linux

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
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func shellCompleteNamespaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()
	nsService := client.NamespaceService()
	nsList, err := nsService.List(ctx)
	if err != nil {
		logrus.Warn(err)
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for _, ns := range nsList {
		candidates = append(candidates, ns)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func shellCompleteSnapshotterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if rootlessutil.IsRootlessParent() {
		_ = rootlessutil.ParentMain()
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer cancel()
	snapshotterPlugins, err := infoutil.GetSnapshotterNames(ctx, client.IntrospectionService())
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	candidates := []string{}
	for _, name := range snapshotterPlugins {
		candidates = append(candidates, name)
	}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

var handledSignals = []os.Signal{
	unix.SIGTERM,
	unix.SIGINT,
	unix.SIGUSR1,
	unix.SIGPIPE,
}

func handleSignals(ctx context.Context, signals chan os.Signal, cancel func()) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case s := <-signals:

				// Do not print message when dealing with SIGPIPE, which may cause
				// nested signals and consume lots of cpu bandwidth.
				if s == unix.SIGPIPE {
					continue
				}

				log.G(ctx).WithField("signal", s).Debug("received signal")
				switch s {
				case unix.SIGUSR1:
					dumpStacks(true)
				default:
					if err := notifyStopping(ctx); err != nil {
						log.G(ctx).WithError(err).Error("notify stopping failed")
					}
					cancel()
					close(done)
					return
				}
			}
		}
	}()
	return done
}
