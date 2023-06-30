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

package containerinspector

import (
	"context"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/typeurl/v2"
	"github.com/sirupsen/logrus"
)

func Inspect(ctx context.Context, container containerd.Container) (*native.Container, error) {
	info, err := container.Info(ctx)
	if err != nil {
		return nil, err
	}
	n := &native.Container{
		Container: info,
	}
	id := container.ID()

	n.Spec, err = typeurl.UnmarshalAny(info.Spec)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Spec")
		return n, nil
	}
	task, err := container.Task(ctx, nil)
	if err != nil {
		// NOTE: NotFound doesn't mean that container hasn't started.
		// In docker/CRI-containerd plugin, the task will be deleted
		// when it exits. So, the status will be "created" for this
		// case.
		if errdefs.IsNotFound(err) {
			n.Process = &native.Process{
				Status: containerd.Status{
					Status: containerd.Created,
				},
			}
		} else {
			logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Task")
		}
		return n, nil
	}
	n.Process = &native.Process{
		Pid: int(task.Pid()),
	}
	st, err := task.Status(ctx)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect Status")
		return n, nil
	}
	n.Process.Status = st
	netNS, err := InspectNetNS(ctx, n.Process.Pid)
	if err != nil {
		logrus.WithError(err).WithField("id", id).Warnf("failed to inspect NetNS")
		return n, nil
	}
	n.Process.NetNS = netNS
	return n, nil
}
