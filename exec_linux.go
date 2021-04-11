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
	"context"
	"io"
	"os"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/pkg/cap"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/nerdctl/pkg/taskutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

func setCapabilities(pspec *specs.Process) {
	if pspec.Capabilities == nil {
		pspec.Capabilities = &specs.LinuxCapabilities{}
	}
	allCaps, err := cap.Current()
	if err != nil {
		return nil, err
	}
	pspec.Capabilities.Bounding = allCaps
	pspec.Capabilities.Permitted = pspec.Capabilities.Bounding
	pspec.Capabilities.Inheritable = pspec.Capabilities.Bounding
	pspec.Capabilities.Effective = pspec.Capabilities.Bounding

	// https://github.com/moby/moby/pull/36466/files
	// > `docker exec --privileged` does not currently disable AppArmor
	// > profiles. Privileged configuration of the container is inherited
}