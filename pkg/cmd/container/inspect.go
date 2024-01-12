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
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/containerinspector"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/dockercompat"
)

// Inspect prints detailed information for each container in `containers`.
func Inspect(ctx context.Context, client *containerd.Client, containers []string, options types.ContainerInspectOptions) error {
	f := &containerInspector{
		mode: options.Mode,
	}

	walker := &containerwalker.ContainerWalker{
		Client:  client,
		OnFound: f.Handler,
	}

	err := walker.WalkAll(ctx, containers, true)
	if len(f.entries) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, f.entries); formatErr != nil {
			log.L.Error(formatErr)
		}
	}
	return err
}

type containerInspector struct {
	mode    string
	entries []interface{}
}

func (x *containerInspector) Handler(ctx context.Context, found containerwalker.Found) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	n, err := containerinspector.Inspect(ctx, found.Container)
	if err != nil {
		return err
	}
	switch x.mode {
	case "native":
		x.entries = append(x.entries, n)
	case "dockercompat":
		d, err := dockercompat.ContainerFromNative(n)
		if err != nil {
			return err
		}
		x.entries = append(x.entries, d)
	default:
		return fmt.Errorf("unknown mode %q", x.mode)
	}
	return nil
}
