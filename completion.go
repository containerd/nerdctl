/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"os"
	"strings"

	"github.com/urfave/cli/v2"
)

var completionCommand = &cli.Command{
	Name:  "completion",
	Usage: "Show shell completion",
	Subcommands: []*cli.Command{
		completionBashCommand,
	},
}

var completionBashCommand = &cli.Command{
	Name:        "bash",
	Usage:       "Show bash completion (use with `source <(nerdctl completion bash)`)",
	Description: "Usage: add `source <(nerdctl completion bash)` to ~/.bash_profile",
	Action:      completionBashAction,
}

func completionBashAction(clicontext *cli.Context) error {
	tmpl := `#!/bin/bash
# Autocompletion enabler for nerdctl.
# Usage: add 'source <(nerdctl completion bash)' to ~/.bash_profile

# _cli_bash_autocomplete is from https://github.com/urfave/cli/blob/v2.3.0/autocomplete/bash_autocomplete (MIT License)
_cli_bash_autocomplete() {
  if [[ "${COMP_WORDS[0]}" != "source" ]]; then
    local cur opts base
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    if [[ "$cur" == "-"* ]]; then
      opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} ${cur} --generate-bash-completion )
    else
      opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} --generate-bash-completion )
    fi
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
  fi
}

complete -o bashdefault -o default -o nospace -F _cli_bash_autocomplete nerdctl
`
	_, err := fmt.Fprint(clicontext.App.Writer, tmpl)
	return err
}

func isFlagCompletionContext() (string, bool) {
	args := os.Args
	// args[len(args)-1] == "--generate-bash-completion"
	// args[len(args)-2] == the current key stroke, e.g. "--ne" for "--net"
	if len(args) <= 2 {
		return "", false
	}
	userTyping := args[len(args)-2]
	if strings.HasPrefix(userTyping, "-") {
		return userTyping, true
	}
	return "", false
}

func defaultBashComplete(clicontext *cli.Context) {
	cli.DefaultCompleteWithFlags(clicontext.Command)(clicontext)
}
