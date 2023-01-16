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

package volume

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/clientutil"
)

func Prune(ctx context.Context, options types.VolumePruneCommandOptions, stdin io.Reader, stdout io.Writer) error {
	if !options.Force {
		var confirm string
		msg := "This will remove all local volumes not used by at least one container."
		msg += "\nAre you sure you want to continue? [y/N] "
		fmt.Fprintf(stdout, "WARNING! %s", msg)
		fmt.Fscanf(stdin, "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}
	volStore, err := Store(options.GOptions.Namespace, options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}
	volumes, err := volStore.List(false)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(ctx, options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	usedVolumes, err := usedVolumes(ctx, containers)
	if err != nil {
		return err
	}
	var removeNames []string // nolint: prealloc
	for _, volume := range volumes {
		if _, ok := usedVolumes[volume.Name]; ok {
			continue
		}
		removeNames = append(removeNames, volume.Name)
	}
	removedNames, err := volStore.Remove(removeNames)
	if err != nil {
		return err
	}
	if len(removedNames) > 0 {
		fmt.Fprintln(stdout, "Deleted Volumes:")
		for _, name := range removedNames {
			fmt.Fprintln(stdout, name)
		}
		fmt.Fprintln(stdout, "")
	}
	return nil
}
