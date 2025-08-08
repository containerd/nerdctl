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
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/image"
)

func ImportCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "import [OPTIONS] file|URL|- [REPOSITORY[:TAG]]",
		Short:             "Import the contents from a tarball to create a filesystem image",
		Args:              cobra.MinimumNArgs(1),
		RunE:              importAction,
		ValidArgsFunction: imageImportShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	cmd.Flags().StringP("message", "m", "", "Set commit message for imported image")
	cmd.Flags().String("platform", "", "Set platform for imported image (e.g., linux/amd64)")
	return cmd
}

func importOptions(cmd *cobra.Command, args []string) (types.ImageImportOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ImageImportOptions{}, err
	}
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return types.ImageImportOptions{}, err
	}
	platform, err := cmd.Flags().GetString("platform")
	if err != nil {
		return types.ImageImportOptions{}, err
	}
	var reference string
	if len(args) > 1 {
		reference = args[1]
	}

	var in io.ReadCloser
	src := args[0]
	switch {
	case src == "-":
		in = io.NopCloser(cmd.InOrStdin())
	case hasHTTPPrefix(src):
		resp, err := http.Get(src)
		if err != nil {
			return types.ImageImportOptions{}, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			defer resp.Body.Close()
			return types.ImageImportOptions{}, fmt.Errorf("failed to download %s: %s", src, resp.Status)
		}
		in = resp.Body
	default:
		f, err := os.Open(src)
		if err != nil {
			return types.ImageImportOptions{}, err
		}
		in = f
	}

	return types.ImageImportOptions{
		Stdout:    cmd.OutOrStdout(),
		Stdin:     in,
		GOptions:  globalOptions,
		Source:    args[0],
		Reference: reference,
		Message:   message,
		Platform:  platform,
	}, nil
}

func importAction(cmd *cobra.Command, args []string) error {
	opt, err := importOptions(cmd, args)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), opt.GOptions.Namespace, opt.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	defer func() {
		if rc, ok := opt.Stdin.(io.ReadCloser); ok {
			_ = rc.Close()
		}
	}()

	name, err := image.Import(ctx, client, opt)
	if err != nil {
		return err
	}
	_, err = cmd.OutOrStdout().Write([]byte(name + "\n"))
	return err
}

func imageImportShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completion.ImageNames(cmd)
}

func hasHTTPPrefix(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
