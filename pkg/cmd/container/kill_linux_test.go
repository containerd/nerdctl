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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
	"gotest.tools/v3/assert"

	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/runtime/restart"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/internal/filesystem"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

// These tests exercise Kill's CNI DEL path with an in-memory containerd service
// and a recording CNI plugin. They do not require a daemon or network privileges.
func TestKillNetworkCleanup(t *testing.T) {
	// A private RootlessKit state directory keeps this test independent of a
	// running rootless daemon. Setenv prevents parallelizing the parent test.
	t.Setenv("ROOTLESSKIT_STATE_DIR", t.TempDir())
	for _, tc := range []struct {
		name, signal, stopSignal                       string
		status                                         tasktypes.Status
		killErr                                        error
		wantCleanup, wantKill, wantResume, wantStopped bool
		wantSignal                                     syscall.Signal
		taskErr, statusErr                             error
		cniError, autoRemove                           bool
		wantErr                                        string
	}{
		{name: "HUP", signal: "HUP", status: tasktypes.Status_RUNNING, wantKill: true, wantSignal: syscall.SIGHUP},
		{name: "USR1", signal: "SIGUSR1", status: tasktypes.Status_RUNNING, wantKill: true, wantSignal: syscall.SIGUSR1},
		{name: "KILL", signal: "SIGKILL", status: tasktypes.Status_RUNNING, wantCleanup: true, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true},
		{name: "TERM", signal: "SIGTERM", status: tasktypes.Status_RUNNING, wantCleanup: true, wantKill: true, wantSignal: syscall.SIGTERM, wantStopped: true},
		{name: "custom stop signal", signal: "SIGUSR1", stopSignal: "SIGUSR1", status: tasktypes.Status_RUNNING, wantCleanup: true, wantKill: true, wantSignal: syscall.SIGUSR1, wantStopped: true},
		{name: "TERM with custom stop signal", signal: "SIGTERM", stopSignal: "SIGUSR1", status: tasktypes.Status_RUNNING, wantKill: true, wantSignal: syscall.SIGTERM},
		{name: "KILL with custom stop signal", signal: "SIGKILL", stopSignal: "SIGUSR1", status: tasktypes.Status_RUNNING, wantCleanup: true, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true},
		{name: "task error", signal: "SIGKILL", taskErr: errors.New("task unavailable"), wantErr: "task unavailable"},
		{name: "status error", signal: "SIGKILL", statusErr: errors.New("status unavailable"), wantErr: "status unavailable"},
		{name: "cleanup error", signal: "SIGKILL", status: tasktypes.Status_RUNNING, cniError: true, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true, wantCleanup: true, wantErr: "unable to cleanup network"},
		{name: "invalid stop signal", signal: "SIGHUP", stopSignal: "invalid", status: tasktypes.Status_RUNNING, wantErr: "invalid signal"},
		{name: "KILL with invalid stop signal", signal: "SIGKILL", stopSignal: "invalid", status: tasktypes.Status_RUNNING, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true, wantCleanup: true},
		{name: "auto removed", signal: "SIGKILL", status: tasktypes.Status_RUNNING, autoRemove: true, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true},
		{name: "created", signal: "SIGKILL", status: tasktypes.Status_CREATED, wantErr: "container is not running"},
		{name: "stopped", signal: "SIGKILL", status: tasktypes.Status_STOPPED, wantErr: "container is not running"},
		{name: "failed signal", signal: "SIGKILL", status: tasktypes.Status_RUNNING, killErr: errors.New("signal failed"), wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true, wantErr: "signal failed"},
		{name: "paused HUP", signal: "SIGHUP", status: tasktypes.Status_PAUSED, wantKill: true, wantSignal: syscall.SIGHUP, wantResume: true},
		{name: "paused KILL", signal: "SIGKILL", status: tasktypes.Status_PAUSED, wantCleanup: true, wantKill: true, wantSignal: syscall.SIGKILL, wantStopped: true, wantResume: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			pluginDir, confDir := filepath.Join(dir, "bin"), filepath.Join(dir, "net.d")
			assert.NilError(t, os.Mkdir(pluginDir, 0o700))
			assert.NilError(t, os.Mkdir(confDir, 0o700))
			marker := filepath.Join(dir, "cni-commands")
			plugin := fmt.Sprintf("#!/bin/sh\ncat >/dev/null\nprintf '%%s\\n' \"$CNI_COMMAND\" >> %q\n", marker)
			if tc.cniError {
				plugin += "exit 1\n"
			}
			assert.NilError(t, filesystem.WriteFile(filepath.Join(pluginDir, "record"), []byte(plugin), 0o700))
			conf := `{"cniVersion":"1.0.0","name":"bridge","plugins":[{"type":"record"}]}`
			assert.NilError(t, filesystem.WriteFile(filepath.Join(confDir, "nerdctl-bridge.conflist"), []byte(conf), 0o600))
			spec, err := typeurl.MarshalAny(&oci.Spec{Annotations: map[string]string{
				labels.Networks: `["bridge"]`, labels.Namespace: "default",
			}})
			assert.NilError(t, err)
			record := containers.Container{ID: "test-container", Spec: spec, Labels: map[string]string{restart.ExplicitlyStoppedLabel: "false"}}
			if tc.stopSignal != "" {
				record.Labels[containerd.StopSignalLabel] = tc.stopSignal
			}
			store := &killTestStore{record: record}
			task := &killTestTasks{status: tc.status, killErr: tc.killErr, taskErr: tc.taskErr, statusErr: tc.statusErr}
			if tc.autoRemove {
				task.onKill = func() { store.removed = true }
			}
			client, err := containerd.New("", containerd.WithServices(containerd.WithContainerStore(store), containerd.WithTaskClient(task)))
			assert.NilError(t, err)
			t.Cleanup(func() { assert.NilError(t, client.Close()) })
			options := types.ContainerKillOptions{KillSignal: tc.signal, Stdout: io.Discard, Stderr: io.Discard,
				GOptions: types.GlobalCommandOptions{DataRoot: filepath.Join(dir, "data"), Address: dir, Namespace: "default", CNIPath: pluginDir, CNINetConfPath: confDir}}
			err = Kill(context.Background(), client, []string{record.ID}, options)
			if tc.wantErr == "" {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.wantErr)
			}
			assert.Equal(t, task.killed, tc.wantKill)
			assert.Equal(t, task.signal, uint32(tc.wantSignal))
			assert.Equal(t, task.resumed, tc.wantResume)
			assert.Equal(t, store.record.Labels[restart.ExplicitlyStoppedLabel], fmt.Sprint(tc.wantStopped))
			commands, err := filesystem.ReadFile(marker)
			if tc.wantCleanup {
				assert.NilError(t, err)
				assert.Equal(t, string(commands), "DEL\n")
			} else {
				assert.Assert(t, errors.Is(err, os.ErrNotExist), "unexpected CNI cleanup: %q (%v)", commands, err)
			}
		})
	}
}

type killTestStore struct {
	containers.Store
	record  containers.Container
	removed bool
}

func (s *killTestStore) Get(context.Context, string) (containers.Container, error) {
	if s.removed {
		return containers.Container{}, errdefs.ErrNotFound
	}
	return s.record, nil
}
func (s *killTestStore) List(context.Context, ...string) ([]containers.Container, error) {
	return []containers.Container{s.record}, nil
}
func (s *killTestStore) Update(_ context.Context, record containers.Container, _ ...string) (containers.Container, error) {
	s.record = record
	return record, nil
}

type killTestTasks struct {
	tasksapi.TasksClient
	status                      tasktypes.Status
	getCalls                    int
	taskErr, statusErr, killErr error
	signal                      uint32
	onKill                      func()
	killed, resumed             bool
}

func (s *killTestTasks) Get(context.Context, *tasksapi.GetRequest, ...grpc.CallOption) (*tasksapi.GetResponse, error) {
	s.getCalls++
	if s.getCalls == 1 && s.taskErr != nil {
		return nil, s.taskErr
	}
	if s.getCalls > 1 && s.statusErr != nil {
		return nil, s.statusErr
	}
	return &tasksapi.GetResponse{Process: &tasktypes.Process{ID: "test-container", Status: s.status}}, nil
}
func (s *killTestTasks) Kill(_ context.Context, req *tasksapi.KillRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
	s.killed = true
	s.signal = req.Signal
	if s.killErr == nil && s.onKill != nil {
		s.onKill()
	}
	if req.Signal == uint32(syscall.SIGKILL) && s.killErr == nil {
		s.status = tasktypes.Status_STOPPED
	}
	return &emptypb.Empty{}, s.killErr
}
func (s *killTestTasks) Resume(context.Context, *tasksapi.ResumeTaskRequest, ...grpc.CallOption) (*emptypb.Empty, error) {
	s.resumed = true
	return &emptypb.Empty{}, nil
}
