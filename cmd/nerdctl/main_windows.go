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
	"unsafe"

	"github.com/containerd/containerd/log"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
)

var (
	handledSignals = []os.Signal{
		windows.SIGTERM,
		windows.SIGINT,
	}
)

func appNeedsRootlessParentMain(cmd *cobra.Command, args []string) bool {
	return false
}

func shellCompleteNamespaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func shellCompleteSnapshotterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
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
	done := make(chan struct{})
	go func() {
		for {
			select {
			case s := <-signals:
				log.G(ctx).WithField("signal", s).Debug("received signal")
				cancel()
				close(done)
				return
			}
		}
	}()
	setupDumpStacks()
	return done
}

func setupDumpStacks() {
	// Windows does not support signals like *nix systems. So instead of
	// trapping on SIGUSR1 to dump stacks, we wait on a Win32 event to be
	// signaled. ACL'd to builtin administrators and local system
	event := "Global\\stackdump-" + fmt.Sprint(os.Getpid())
	ev, _ := windows.UTF16PtrFromString(event)
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;BA)(A;;GA;;;SY)")
	if err != nil {
		logrus.Errorf("failed to get security descriptor for debug stackdump event %s: %s", event, err.Error())
		return
	}
	var sa windows.SecurityAttributes
	sa.Length = uint32(unsafe.Sizeof(sa))
	sa.InheritHandle = 1
	sa.SecurityDescriptor = sd
	h, err := windows.CreateEvent(&sa, 0, 0, ev)
	if h == 0 || err != nil {
		logrus.Errorf("failed to create debug stackdump event %s: %s", event, err.Error())
		return
	}
	go func() {
		logrus.Debugf("Stackdump - waiting signal at %s", event)
		for {
			windows.WaitForSingleObject(h, windows.INFINITE)
			dumpStacks(true)
		}
	}()
}
