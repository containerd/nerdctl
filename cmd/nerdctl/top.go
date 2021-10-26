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

/*
   Portions from:
   - https://github.com/moby/moby/blob/v20.10.6/api/types/container/container_top.go
   - https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go
   Copyright (C) The Moby authors.
   Licensed under the Apache License, Version 2.0
*/

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/rootlessutil"

	"github.com/spf13/cobra"
)

// ContainerTopOKBody is from https://github.com/moby/moby/blob/v20.10.6/api/types/container/container_top.go
//
// ContainerTopOKBody OK response to ContainerTop operation
type ContainerTopOKBody struct {

	// Each process running in the container, where each is process
	// is an array of values corresponding to the titles.
	//
	// Required: true
	Processes [][]string `json:"Processes"`

	// The ps column titles
	// Required: true
	Titles []string `json:"Titles"`
}

func newTopCommand() *cobra.Command {
	var topCommand = &cobra.Command{
		Use:               "top CONTAINER [ps OPTIONS]",
		Args:              cobra.MinimumNArgs(1),
		Short:             "Display the running processes of a container",
		RunE:              topAction,
		ValidArgsFunction: topShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	topCommand.Flags().SetInterspersed(false)
	return topCommand
}

func topAction(cmd *cobra.Command, args []string) error {

	if len(args) < 1 {
		return fmt.Errorf("requires at least 1 argument")
	}

	// NOTE: rootless container does not rely on cgroupv1.
	// more details about possible ways to resolve this concern: #223
	if rootlessutil.IsRootless() && infoutil.CgroupsVersion() == "1" {
		return fmt.Errorf("top requires cgroup v2 for rootless containers, see https://rootlesscontaine.rs/getting-started/common/cgroup2/")
	}

	cgroupManager, err := defaults.GetglobalString(cmd, "cgroup-manager")
	if err != nil {
		return err
	}
	if cgroupManager == "none" {
		return errors.New("cgroup manager must not be \"none\"")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if err := containerTop(ctx, cmd, client, found.Container.ID(), strings.Join(args[1:], " ")); err != nil {
				return err
			}
			return nil
		},
	}

	n, err := walker.Walk(ctx, args[0])
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", args[0])
	}
	return nil
}

// appendProcess2ProcList is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L49-L55
func appendProcess2ProcList(procList *ContainerTopOKBody, fields []string) {
	// Make sure number of fields equals number of header titles
	// merging "overhanging" fields
	process := fields[:len(procList.Titles)-1]
	process = append(process, strings.Join(fields[len(procList.Titles)-1:], " "))
	procList.Processes = append(procList.Processes, process)
}

// psPidsArg is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L119-L131
//
// psPidsArg converts a slice of PIDs to a string consisting
// of comma-separated list of PIDs prepended by "-q".
// For example, psPidsArg([]uint32{1,2,3}) returns "-q1,2,3".
func psPidsArg(pids []uint32) string {
	b := []byte{'-', 'q'}
	for i, p := range pids {
		b = strconv.AppendUint(b, uint64(p), 10)
		if i < len(pids)-1 {
			b = append(b, ',')
		}
	}
	return string(b)
}

// validatePSArgs is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L19-L35
func validatePSArgs(psArgs string) error {
	// NOTE: \\s does not detect unicode whitespaces.
	// So we use fieldsASCII instead of strings.Fields in parsePSOutput.
	// See https://github.com/docker/docker/pull/24358
	// nolint: gosimple
	re := regexp.MustCompile("\\s+([^\\s]*)=\\s*(PID[^\\s]*)")
	for _, group := range re.FindAllStringSubmatch(psArgs, -1) {
		if len(group) >= 3 {
			k := group[1]
			v := group[2]
			if k != "pid" {
				return fmt.Errorf("specifying \"%s=%s\" is not allowed", k, v)
			}
		}
	}
	return nil
}

// fieldsASCII is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L37-L47
//
// fieldsASCII is similar to strings.Fields but only allows ASCII whitespaces
func fieldsASCII(s string) []string {
	fn := func(r rune) bool {
		switch r {
		case '\t', '\n', '\f', '\r', ' ':
			return true
		}
		return false
	}
	return strings.FieldsFunc(s, fn)
}

// hasPid is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L57-L64
func hasPid(procs []uint32, pid int) bool {
	for _, p := range procs {
		if int(p) == pid {
			return true
		}
	}
	return false
}

// parsePSOutput is from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L66-L117
func parsePSOutput(output []byte, procs []uint32) (*ContainerTopOKBody, error) {
	procList := &ContainerTopOKBody{}

	lines := strings.Split(string(output), "\n")
	procList.Titles = fieldsASCII(lines[0])

	pidIndex := -1
	for i, name := range procList.Titles {
		if name == "PID" {
			pidIndex = i
			break
		}
	}
	if pidIndex == -1 {
		return nil, fmt.Errorf("Couldn't find PID field in ps output")
	}

	// loop through the output and extract the PID from each line
	// fixing #30580, be able to display thread line also when "m" option used
	// in "docker top" client command
	preContainedPidFlag := false
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		fields := fieldsASCII(line)

		var (
			p   int
			err error
		)

		if fields[pidIndex] == "-" {
			if preContainedPidFlag {
				appendProcess2ProcList(procList, fields)
			}
			continue
		}
		p, err = strconv.Atoi(fields[pidIndex])
		if err != nil {
			return nil, fmt.Errorf("Unexpected pid '%s': %s", fields[pidIndex], err)
		}

		if hasPid(procs, p) {
			preContainedPidFlag = true
			appendProcess2ProcList(procList, fields)
			continue
		}
		preContainedPidFlag = false
	}
	return procList, nil
}

// containerTop was inspired from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L133-L189
//
// ContainerTop lists the processes running inside of the given
// container by calling ps with the given args, or with the flags
// "-ef" if no args are given.  An error is returned if the container
// is not found, or is not running, or if there are any problems
// running ps, or parsing the output.
func containerTop(ctx context.Context, cmd *cobra.Command, client *containerd.Client, id string, psArgs string) error {
	if psArgs == "" {
		psArgs = "-ef"
	}

	if err := validatePSArgs(psArgs); err != nil {
		return err
	}

	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	if status.Status != containerd.Running {
		return nil
	}

	//TO DO handle restarting case: wait for container to restart and then launch top command

	procs, err := task.Pids(ctx)
	if err != nil {
		return err
	}

	psList := make([]uint32, 0, len(procs))
	for _, ps := range procs {
		psList = append(psList, ps.Pid)
	}

	args := strings.Split(psArgs, " ")
	pids := psPidsArg(psList)
	output, err := exec.Command("ps", append(args, pids)...).Output()
	if err != nil {
		// some ps options (such as f) can't be used together with q,
		// so retry without it
		output, err = exec.Command("ps", args...).Output()
		if err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				// first line of stderr shows why ps failed
				line := bytes.SplitN(ee.Stderr, []byte{'\n'}, 2)
				if len(line) > 0 && len(line[0]) > 0 {
					return errors.New(string(line[0]))
				}
			}
			return nil
		}
	}
	procList, err := parsePSOutput(output, psList)
	if err != nil {
		return err
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(procList.Titles, "\t"))

	for _, proc := range procList.Processes {
		fmt.Fprintln(w, strings.Join(proc, "\t"))
	}

	return w.Flush()
}

func topShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show running container names
	statusFilterFn := func(st containerd.ProcessStatus) bool {
		return st == containerd.Running
	}
	return shellCompleteContainerNames(cmd, statusFilterFn)
}
