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

package helpers

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

// UnknownSubcommandAction is needed to let `nerdctl system non-existent-command` fail
// https://github.com/containerd/nerdctl/issues/487
//
// Ideally this should be implemented in Cobra itself.
func UnknownSubcommandAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	// The output mimics https://github.com/spf13/cobra/blob/v1.2.1/command.go#L647-L662
	msg := fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.Name())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += fmt.Sprintf("\t%v\n", s)
		}
	}
	return errors.New(msg)
}

// IsExactArgs returns an error if there is not the exact number of args
func IsExactArgs(number int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == number {
			return nil
		}
		return fmt.Errorf(
			"%q requires exactly %d %s.\nSee '%s --help'.\n\nUsage:  %s\n\n%s",
			cmd.CommandPath(),
			number,
			"argument(s)",
			cmd.CommandPath(),
			cmd.UseLine(),
			cmd.Short,
		)
	}
}
