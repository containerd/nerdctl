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
	"github.com/spf13/cobra"
)

func newRestartCommand() *cobra.Command {
	var restartCommand = &cobra.Command{
		Use:               "restart [flags] CONTAINER [CONTAINER, ...]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Restart one or more running containers",
		RunE:              restartAction,
		ValidArgsFunction: startShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	restartCommand.Flags().StringP("time", "t", "10", "Seconds to wait for stop before killing it")
	return restartCommand
}

func restartAction(cmd *cobra.Command, args []string) error {
	err := stopAction(cmd, args)
	if err != nil {
		return err
	}
	err = startAction(cmd, args)
	if err != nil {
		return err
	}
	return nil
}
