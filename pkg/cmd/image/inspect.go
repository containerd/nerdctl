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

package image

import (
	"context"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imageinspector"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/sirupsen/logrus"
)

// Inspect prints detailed information of each image in `images`.
func Inspect(ctx context.Context, client *containerd.Client, images []string, options types.ImageInspectOptions) error {
	f := &imageInspector{
		mode: options.Mode,
	}
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			n, err := imageinspector.Inspect(ctx, client, found.Image)
			if err != nil {
				return err
			}
			switch f.mode {
			case "native":
				f.entries = append(f.entries, n)
			case "dockercompat":
				d, err := dockercompat.ImageFromNative(n)
				if err != nil {
					return err
				}
				f.entries = append(f.entries, d)
			default:
				return fmt.Errorf("unknown mode %q", f.mode)
			}
			return nil
		},
	}

	err := walker.WalkAll(ctx, images, true)
	if len(f.entries) > 0 {
		if formatErr := formatter.FormatSlice(options.Format, options.Stdout, f.entries); formatErr != nil {
			logrus.Error(formatErr)
		}
	}
	return err
}

type imageInspector struct {
	mode    string
	entries []interface{}
}
