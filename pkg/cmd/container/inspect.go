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
	"time"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
)

func Inspect(ctx context.Context, options types.ContainerInspectCommandOptions, stdout io.Writer) error {
	client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	f := &containerInspector{
		mode: options.Mode,
	}

	walker := &containerwalker.ContainerWalker{
		Client:  client,
		OnFound: f.Handler,
	}

	var errs []error
	for _, req := range options.Containers {
		n, err := walker.Walk(ctx, req)
		if err != nil {
			errs = append(errs, err)
		} else if n == 0 {
			errs = append(errs, fmt.Errorf("no such container: %s", req))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d errors: %v", len(errs), errs)
	}

	return formatter.FormatSlice(options.Format, stdout, f.entries)
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
