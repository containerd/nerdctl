//go:build linux || darwin || freebsd || netbsd || openbsd

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
   NOTICE: https://github.com/moby/moby/blob/v20.10.6/NOTICE
*/

package container

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/tabwriter"

	containerd "github.com/containerd/containerd/v2/client"
)

// containerTop was inspired from https://github.com/moby/moby/blob/v20.10.6/daemon/top_unix.go#L133-L189
//
// ContainerTop lists the processes running inside of the given
// container by calling ps with the given args, or with the flags
// "-ef" if no args are given.  An error is returned if the container
// is not found, or is not running, or if there are any problems
// running ps, or parsing the output.
// procList *ContainerTopOKBody
func containerTop(ctx context.Context, stdio io.Writer, client *containerd.Client, id string, psArgs string) error {
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

	w := tabwriter.NewWriter(stdio, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(procList.Titles, "\t"))

	for _, proc := range procList.Processes {
		fmt.Fprintln(w, strings.Join(proc, "\t"))
	}

	return w.Flush()
}
