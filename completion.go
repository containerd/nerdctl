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
	"os"
	"strings"

	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/netutil"
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

# _nerdctl_bash_autocomplete is forked from https://github.com/urfave/cli/blob/v2.3.0/autocomplete/bash_autocomplete (MIT License)
_nerdctl_bash_autocomplete() {
  if [[ "${COMP_WORDS[0]}" != "source" ]]; then
    local cur opts base
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    local args="${COMP_WORDS[@]:0:$COMP_CWORD}"
    # make {"nerdctl", "--namespace", "=", "foo"} into {"nerdctl", "--namespace=foo"}
    args="$(echo $args | sed -e 's/ = /=/g')"
    if [[ "$cur" == "-"* ]]; then
      opts=$( ${args} ${cur} --generate-bash-completion )
    else
      opts=$( ${args} --generate-bash-completion )
    fi
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
  fi
}

complete -o bashdefault -o default -o nospace -F _nerdctl_bash_autocomplete nerdctl
`
	_, err := fmt.Fprint(clicontext.App.Writer, tmpl)
	return err
}

type completionContext struct {
	flagName       string
	flagTakesValue bool
	boring         bool // should call the default completer
}

func parseCompletionContext(clicontext *cli.Context) (coco completionContext) {
	args := os.Args // not clicontext.Args().Slice()
	// args[len(args)-2] == the current key stroke, e.g. "--net"
	if len(args) <= 2 {
		coco.boring = true
		return
	}
	userTyping := args[len(args)-2]
	if strings.HasPrefix(userTyping, "-") {
		flagNameCandidate := strings.TrimLeft(userTyping, "-")
		if !strings.HasPrefix(userTyping, "--") {
			// when userTyping is like "-it", we take "-t"
			flagNameCandidate = string(userTyping[len(userTyping)-1])
		}
		isFlagName, flagTakesValue := checkFlagName(clicontext, flagNameCandidate)
		if !isFlagName {
			coco.boring = true
			return
		}
		coco.flagName = flagNameCandidate
		coco.flagTakesValue = flagTakesValue
	}
	return
}

// checkFlagName returns (isFlagName, flagTakesValue)
func checkFlagName(clicontext *cli.Context, flagName string) (bool, bool) {
	visibleFlags := clicontext.App.VisibleFlags()
	if clicontext.Command != nil && clicontext.Command.Name != "" {
		visibleFlags = clicontext.Command.VisibleFlags()
	}
	for _, visi := range visibleFlags {
		for _, visiName := range visi.Names() {
			if visiName == flagName {
				type valueTaker interface {
					TakesValue() bool
				}
				vt, ok := visi.(valueTaker)
				if !ok {
					return true, false
				}
				return true, vt.TakesValue()
			}
		}
	}
	return false, false
}

func defaultBashComplete(clicontext *cli.Context) {
	if clicontext.Command == nil {
		cli.DefaultCompleteWithFlags(nil)(clicontext)
	}

	// Dirty hack to hide global app flags such as "--namespace" , "--cgroup-manager"
	dummyApp := cli.NewApp()
	dummyApp.Writer = clicontext.App.Writer
	dummyCliContext := cli.NewContext(dummyApp, nil, nil)
	cli.DefaultCompleteWithFlags(clicontext.Command)(dummyCliContext)
}

func bashCompleteImageNames(clicontext *cli.Context) {
	w := clicontext.App.Writer
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()

	imageList, err := client.ImageService().List(ctx, "")
	if err != nil {
		return
	}
	for _, img := range imageList {
		fmt.Fprintln(w, img.Name)
	}
}

func bashCompleteContainerNames(clicontext *cli.Context) {
	w := clicontext.App.Writer
	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return
	}
	defer cancel()
	containers, err := client.Containers(ctx)
	if err != nil {
		return
	}
	for _, c := range containers {
		lab, err := c.Labels(ctx)
		if err != nil {
			continue
		}
		name := lab[labels.Name]
		if name != "" {
			fmt.Fprintln(w, name)
			continue
		}
		fmt.Fprintln(w, c.ID())
	}
}

// bashCompleteNetworkNames includes {"bridge","host","none"}
func bashCompleteNetworkNames(clicontext *cli.Context, exclude []string) {
	excludeMap := make(map[string]struct{}, len(exclude))
	for _, ex := range exclude {
		excludeMap[ex] = struct{}{}
	}

	// To avoid nil panic during clicontext.String(),
	// it seems we have to use globalcontext.String()
	lineage := clicontext.Lineage()
	if len(lineage) < 2 {
		return
	}
	globalContext := lineage[len(lineage)-2]
	e := &netutil.CNIEnv{
		Path:        globalContext.String("cni-path"),
		NetconfPath: globalContext.String("cni-netconfpath"),
	}

	configLists, err := netutil.ConfigLists(e)
	if err != nil {
		return
	}
	w := clicontext.App.Writer
	for _, configList := range configLists {
		if _, ok := excludeMap[configList.Name]; !ok {
			fmt.Fprintln(w, configList.Name)
		}
	}
	for _, s := range []string{"host", "none"} {
		if _, ok := excludeMap[s]; !ok {
			fmt.Fprintln(w, s)
		}
	}
}

func bashCompleteVolumeNames(clicontext *cli.Context) {
	w := clicontext.App.Writer
	vols, err := getVolumes(clicontext)
	if err != nil {
		return
	}
	for _, v := range vols {
		fmt.Fprintln(w, v.Name)
	}
}
