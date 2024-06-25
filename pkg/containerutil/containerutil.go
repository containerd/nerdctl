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

package containerutil

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/containerd/runtime/restart"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/ipcutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/labels/k8slabels"
	"github.com/containerd/nerdctl/v2/pkg/nsutil"
	"github.com/containerd/nerdctl/v2/pkg/portutil"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/signalutil"
	"github.com/containerd/nerdctl/v2/pkg/taskutil"
	"github.com/moby/sys/signal"
	"github.com/opencontainers/runtime-spec/specs-go"
)

// PrintHostPort writes to `writer` the public (HostIP:HostPort) of a given `containerPort/protocol` in a container.
// if `containerPort < 0`, it writes all public ports of the container.
func PrintHostPort(ctx context.Context, writer io.Writer, container containerd.Container, containerPort int, proto string) error {
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	ports, err := portutil.ParsePortsLabel(l)
	if err != nil {
		return err
	}

	if containerPort < 0 {
		for _, p := range ports {
			fmt.Fprintf(writer, "%d/%s -> %s:%d\n", p.ContainerPort, p.Protocol, p.HostIP, p.HostPort)
		}
		return nil
	}

	for _, p := range ports {
		if p.ContainerPort == int32(containerPort) && strings.ToLower(p.Protocol) == proto {
			fmt.Fprintf(writer, "%s:%d\n", p.HostIP, p.HostPort)
			return nil
		}
	}
	return fmt.Errorf("no public port %d/%s published for %q", containerPort, proto, container.ID())
}

// ContainerStatus returns the container's status from its task.
func ContainerStatus(ctx context.Context, c containerd.Container) (containerd.Status, error) {
	// Just in case, there is something wrong in server.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	task, err := c.Task(ctx, nil)
	if err != nil {
		return containerd.Status{}, err
	}

	return task.Status(ctx)
}

// ContainerNetNSPath returns the netns path of a container.
func ContainerNetNSPath(ctx context.Context, c containerd.Container) (string, error) {
	task, err := c.Task(ctx, nil)
	if err != nil {
		return "", err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return "", err
	}
	if status.Status != containerd.Running {
		return "", fmt.Errorf("invalid target container: %s, should be running", c.ID())
	}
	return fmt.Sprintf("/proc/%d/ns/net", task.Pid()), nil
}

// UpdateStatusLabel updates the "containerd.io/restart.status"
// label of the container according to the value of restart desired status.
func UpdateStatusLabel(ctx context.Context, container containerd.Container, status containerd.ProcessStatus) error {
	opt := containerd.WithAdditionalContainerLabels(map[string]string{
		restart.StatusLabel: string(status),
	})
	return container.Update(ctx, containerd.UpdateContainerOpts(opt))
}

// UpdateExplicitlyStoppedLabel updates the "containerd.io/restart.explicitly-stopped"
// label of the container according to the value of explicitlyStopped.
func UpdateExplicitlyStoppedLabel(ctx context.Context, container containerd.Container, explicitlyStopped bool) error {
	opt := containerd.WithAdditionalContainerLabels(map[string]string{
		restart.ExplicitlyStoppedLabel: strconv.FormatBool(explicitlyStopped),
	})
	return container.Update(ctx, containerd.UpdateContainerOpts(opt))
}

// UpdateErrorLabel updates the "nerdctl/error"
// label of the container according to the container error.
func UpdateErrorLabel(ctx context.Context, container containerd.Container, err error) error {
	opt := containerd.WithAdditionalContainerLabels(map[string]string{
		labels.Error: err.Error(),
	})
	return container.Update(ctx, containerd.UpdateContainerOpts(opt))
}

// WithBindMountHostProcfs replaces procfs mount with rbind.
// Required for --pid=host on rootless.
//
// https://github.com/moby/moby/pull/41893/files
// https://github.com/containers/podman/blob/v3.0.0-rc1/pkg/specgen/generate/oci.go#L248-L257
func WithBindMountHostProcfs(_ context.Context, _ oci.Client, _ *containers.Container, s *oci.Spec) error {
	for i, m := range s.Mounts {
		if path.Clean(m.Destination) == "/proc" {
			newM := specs.Mount{
				Destination: "/proc",
				Type:        "bind",
				Source:      "/proc",
				Options:     []string{"rbind", "nosuid", "noexec", "nodev"},
			}
			s.Mounts[i] = newM
		}
	}

	// Remove ReadonlyPaths for /proc/*
	newROP := s.Linux.ReadonlyPaths[:0]
	for _, x := range s.Linux.ReadonlyPaths {
		x = path.Clean(x)
		if !strings.HasPrefix(x, "/proc/") {
			newROP = append(newROP, x)
		}
	}
	s.Linux.ReadonlyPaths = newROP
	return nil
}

// GenerateSharingPIDOpts returns the oci.SpecOpts that shares the host linux namespace from `targetCon`
// If `targetCon` doesn't have a `PIDNamespace`, a new one is generated from its `Pid`.
func GenerateSharingPIDOpts(ctx context.Context, targetCon containerd.Container) ([]oci.SpecOpts, error) {
	opts := make([]oci.SpecOpts, 0)

	task, err := targetCon.Task(ctx, nil)
	if err != nil {
		return nil, err
	}
	status, err := task.Status(ctx)
	if err != nil {
		return nil, err
	}

	if status.Status != containerd.Running {
		return nil, fmt.Errorf("shared container is not running")
	}

	spec, err := targetCon.Spec(ctx)
	if err != nil {
		return nil, err
	}

	isHost := true
	for _, n := range spec.Linux.Namespaces {
		if n.Type == specs.PIDNamespace {
			isHost = false
		}
	}
	if isHost {
		opts = append(opts, oci.WithHostNamespace(specs.PIDNamespace))
		if rootlessutil.IsRootless() {
			opts = append(opts, WithBindMountHostProcfs)
		}
	} else {
		ns := specs.LinuxNamespace{
			Type: specs.PIDNamespace,
			Path: fmt.Sprintf("/proc/%d/ns/pid", task.Pid()),
		}
		opts = append(opts, oci.WithLinuxNamespace(ns))
	}
	return opts, nil
}

// Start starts `container` with `attach` flag. If `attach` is true, it will attach to the container's stdio.
func Start(ctx context.Context, container containerd.Container, flagA bool, client *containerd.Client, detachKeys string) (err error) {
	// defer the storage of start error in the dedicated label
	defer func() {
		if err != nil {
			UpdateErrorLabel(ctx, container, err)
		}
	}()
	lab, err := container.Labels(ctx)
	if err != nil {
		return err
	}

	if err := ReconfigNetContainer(ctx, container, client, lab); err != nil {
		return err
	}

	if err := ReconfigPIDContainer(ctx, container, client, lab); err != nil {
		return err
	}

	if err := ReconfigIPCContainer(ctx, container, client, lab); err != nil {
		return err
	}

	process, err := container.Spec(ctx)
	if err != nil {
		return err
	}
	flagT := process.Process.Terminal
	var con console.Console
	if flagA && flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}

	logURI := lab[labels.LogURI]
	namespace := lab[labels.Namespace]
	cStatus := formatter.ContainerStatus(ctx, container)
	if cStatus == "Up" {
		log.G(ctx).Warnf("container %s is already running", container.ID())
		return nil
	}

	_, restartPolicyExist := lab[restart.PolicyLabel]
	if restartPolicyExist {
		if err := UpdateStatusLabel(ctx, container, containerd.Running); err != nil {
			return err
		}
	}

	if err := UpdateExplicitlyStoppedLabel(ctx, container, false); err != nil {
		return err
	}
	if oldTask, err := container.Task(ctx, nil); err == nil {
		if _, err := oldTask.Delete(ctx); err != nil {
			log.G(ctx).WithError(err).Debug("failed to delete old task")
		}
	}
	detachC := make(chan struct{})
	task, err := taskutil.NewTask(ctx, client, container, flagA, false, flagT, true, con, logURI, detachKeys, namespace, detachC)
	if err != nil {
		return err
	}

	if err := task.Start(ctx); err != nil {
		return err
	}
	if !flagA {
		return nil
	}
	if flagA && flagT {
		if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
			log.G(ctx).WithError(err).Error("console resize")
		}
	}
	sigc := signalutil.ForwardAllSignals(ctx, task)
	defer signalutil.StopCatch(sigc)

	statusC, err := task.Wait(ctx)
	if err != nil {
		return err
	}
	select {
	// io.Wait() would return when either 1) the user detaches from the container OR 2) the container is about to exit.
	//
	// If we replace the `select` block with io.Wait() and
	// directly use task.Status() to check the status of the container after io.Wait() returns,
	// it can still be running even though the container is about to exit (somehow especially for Windows).
	//
	// As a result, we need a separate detachC to distinguish from the 2 cases mentioned above.
	case <-detachC:
		io := task.IO()
		if io == nil {
			return errors.New("got a nil IO from the task")
		}
		io.Wait()
	case status := <-statusC:
		code, _, err := status.Result()
		if err != nil {
			return err
		}
		if code != 0 {
			return errutil.NewExitCoderErr(int(code))
		}
	}
	return nil
}

// Stop stops `container` by sending SIGTERM. If the container is not stopped after `timeout`, it sends a SIGKILL.
func Stop(ctx context.Context, container containerd.Container, timeout *time.Duration) (err error) {
	// defer the storage of stop error in the dedicated label
	defer func() {
		if err != nil {
			UpdateErrorLabel(ctx, container, err)
		}
	}()
	if err := UpdateExplicitlyStoppedLabel(ctx, container, true); err != nil {
		return err
	}

	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	ipc, err := ipcutil.DecodeIPCLabel(l[labels.IPC])
	if err != nil {
		return err
	}
	// defer umount
	defer func() {
		if err := ipcutil.CleanUp(ipc); err != nil {
			log.G(ctx).Warnf("failed to clean up IPC container %s: %s", container.ID(), err)
		}
	}()

	if timeout == nil {
		t, ok := l[labels.StopTimeout]
		if !ok {
			// Default is 10 seconds.
			t = "10"
		}
		td, err := time.ParseDuration(t + "s")
		if err != nil {
			return err
		}
		timeout = &td
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	paused := false

	switch status.Status {
	case containerd.Created, containerd.Stopped:
		return nil
	case containerd.Paused, containerd.Pausing:
		paused = true
	default:
	}

	// NOTE: ctx is main context so that it's ok to use for task.Wait().
	exitCh, err := task.Wait(ctx)
	if err != nil {
		return err
	}

	if *timeout > 0 {
		sig, err := signal.ParseSignal("SIGTERM")
		if err != nil {
			return err
		}
		if stopSignal, ok := l[containerd.StopSignalLabel]; ok {
			sig, err = signal.ParseSignal(stopSignal)
			if err != nil {
				return err
			}
		}

		if err := task.Kill(ctx, sig); err != nil {
			return err
		}

		// signal will be sent once resume is finished
		if paused {
			if err := task.Resume(ctx); err != nil {
				log.G(ctx).Warnf("Cannot unpause container %s: %s", container.ID(), err)
			} else {
				// no need to do it again when send sigkill signal
				paused = false
			}
		}

		sigtermCtx, sigtermCtxCancel := context.WithTimeout(ctx, *timeout)
		defer sigtermCtxCancel()

		err = waitContainerStop(sigtermCtx, exitCh, container.ID())
		if err == nil {
			return nil
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	sig, err := signal.ParseSignal("SIGKILL")
	if err != nil {
		return err
	}

	if err := task.Kill(ctx, sig); err != nil {
		return err
	}

	// signal will be sent once resume is finished
	if paused {
		if err := task.Resume(ctx); err != nil {
			log.G(ctx).Warnf("Cannot unpause container %s: %s", container.ID(), err)
		}
	}
	return waitContainerStop(ctx, exitCh, container.ID())
}

func waitContainerStop(ctx context.Context, exitCh <-chan containerd.ExitStatus, id string) error {
	select {
	case <-ctx.Done():
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("wait container %v: %w", id, err)
		}
		return nil
	case status := <-exitCh:
		return status.Error()
	}
}

// Pause pauses a container by its id.
func Pause(ctx context.Context, client *containerd.Client, id string) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	switch status.Status {
	case containerd.Paused:
		return fmt.Errorf("container %s is already paused", id)
	case containerd.Created, containerd.Stopped:
		return fmt.Errorf("container %s is not running", id)
	default:
		return task.Pause(ctx)
	}
}

// Unpause unpauses a container by its id.
func Unpause(ctx context.Context, client *containerd.Client, id string) error {
	container, err := client.LoadContainer(ctx, id)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, cio.Load)
	if err != nil {
		return err
	}

	status, err := task.Status(ctx)
	if err != nil {
		return err
	}

	switch status.Status {
	case containerd.Paused:
		return task.Resume(ctx)
	default:
		return fmt.Errorf("container %s is not paused", id)
	}
}

// ContainerStateDirPath returns the path to the Nerdctl-managed state directory for the container with the given ID.
func ContainerStateDirPath(ns, dataStore, id string) (string, error) {
	if err := nsutil.ValidateNamespaceName(ns); err != nil {
		return "", fmt.Errorf("invalid namespace name %q for determining state dir of container %q: %s", ns, id, err)
	}
	return filepath.Join(dataStore, "containers", ns, id), nil
}

// ContainerVolume is a struct representing a volume in a container.
type ContainerVolume struct {
	Type        string
	Name        string
	Source      string
	Destination string
	Mode        string
	RW          bool
	Propagation string
}

// GetContainerVolumes is a function that returns a slice of containerVolume pointers.
// It accepts a map of container labels as input, where key is the label name and value is its associated value.
// The function iterates over the predefined volume labels (AnonymousVolumes and Mounts)
// and for each, it checks if the labels exists in the provided container labels.
// If yes, it decodes the label value from JSON format and appends the volumes to the result.
// In case of error during decoding, it logs the error and continues to the next label.
func GetContainerVolumes(containerLabels map[string]string) []*ContainerVolume {
	var vols []*ContainerVolume
	volLabels := []string{labels.AnonymousVolumes, labels.Mounts}
	for _, volLabel := range volLabels {
		names, ok := containerLabels[volLabel]
		if !ok {
			continue
		}
		var (
			volumes []*ContainerVolume
			err     error
		)
		if volLabel == labels.Mounts {
			err = json.Unmarshal([]byte(names), &volumes)
		}
		if volLabel == labels.AnonymousVolumes {
			var anonymous []string
			err = json.Unmarshal([]byte(names), &anonymous)
			for _, anony := range anonymous {
				volumes = append(volumes, &ContainerVolume{Name: anony})
			}

		}
		if err != nil {
			log.L.Warn(err)
		}
		vols = append(vols, volumes...)
	}
	return vols
}

func GetContainerName(containerLabels map[string]string) string {
	if name, ok := containerLabels[labels.Name]; ok {
		return name
	}

	if ns, ok := containerLabels[k8slabels.PodNamespace]; ok {
		if podName, ok := containerLabels[k8slabels.PodName]; ok {
			if containerName, ok := containerLabels[k8slabels.ContainerName]; ok {
				// Container
				return fmt.Sprintf("k8s://%s/%s/%s", ns, podName, containerName)
			}
			// Pod sandbox
			return fmt.Sprintf("k8s://%s/%s", ns, podName)
		}
	}
	return ""
}
