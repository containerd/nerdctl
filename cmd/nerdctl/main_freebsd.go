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
	"os"

	"github.com/containerd/containerd/log"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

var handledSignals = []os.Signal{
	unix.SIGTERM,
	unix.SIGINT,
	unix.SIGUSR1,
	unix.SIGPIPE,
}

func appNeedsRootlessParentMain(cmd *cobra.Command, args []string) bool {
	return false
}

func shellCompleteCgroupManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func addApparmorCommand(rootCmd *cobra.Command) {
	// NOP
}

func addCpCommand(rootCmd *cobra.Command) {
	// NOP
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
					cancel()
					close(done)
					return
				}
			}
		}
	}()
	return done
}
