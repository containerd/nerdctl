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

package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/typeurl/v2"
	"github.com/docker/go-units"
)

// containerTop was inspired from https://github.com/moby/moby/blob/master/daemon/top_windows.go
//
// ContainerTop lists the processes running inside of the given
// container. An error is returned if the container
// is not found, or is not running.
func containerTop(ctx context.Context, stdio io.Writer, client *containerd.Client, id string, psArgs string) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}
	processes, err := task.Pids(ctx)
	if err != nil {
		return err
	}
	procList := &ContainerTopOKBody{}
	procList.Titles = []string{"Name", "PID", "CPU", "Private Working Set"}

	for _, j := range processes {
		var info options.ProcessDetails
		err = typeurl.UnmarshalTo(j.Info, &info)
		if err != nil {
			return err
		}
		d := time.Duration((info.KernelTime_100Ns + info.UserTime_100Ns) * 100) // Combined time in nanoseconds
		procList.Processes = append(procList.Processes, []string{
			info.ImageName,
			fmt.Sprint(info.ProcessID),
			fmt.Sprintf("%02d:%02d:%02d.%03d", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60, int(d.Nanoseconds()/1000000)%1000),
			units.HumanSize(float64(info.MemoryWorkingSetPrivateBytes))})

	}

	w := tabwriter.NewWriter(os.Stdout, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, strings.Join(procList.Titles, "\t"))

	for _, proc := range procList.Processes {
		fmt.Fprintln(w, strings.Join(proc, "\t"))
	}
	w.Flush()

	return nil
}
