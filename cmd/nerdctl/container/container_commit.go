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
	"errors"

	"github.com/spf13/cobra"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
)

func CommitCommand() *cobra.Command {
	var cmd = &cobra.Command{
		Use:               "commit [flags] CONTAINER REPOSITORY[:TAG]",
		Short:             "Create a new image from a container's changes",
		Args:              helpers.IsExactArgs(2),
		RunE:              commitAction,
		ValidArgsFunction: commitShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	cmd.Flags().StringP("author", "a", "", `Author (e.g., "nerdctl contributor <nerdctl-dev@example.com>")`)
	cmd.Flags().StringP("message", "m", "", "Commit message")
	cmd.Flags().StringArrayP("change", "c", nil, "Apply Dockerfile instruction to the created image (supported directives: [CMD, ENTRYPOINT])")
	cmd.Flags().BoolP("pause", "p", true, "Pause container during commit")
	cmd.Flags().StringP("compression", "", "gzip", "commit compression algorithm (zstd or gzip)")
	cmd.Flags().String("format", "docker", "Format of the committed image (docker or oci)")
	cmd.Flags().Bool("estargz", false, "Convert the committed layer to eStargz for lazy pulling")
	cmd.Flags().Int("estargz-compression-level", 9, "eStargz compression level (1-9)")
	cmd.Flags().Int("estargz-chunk-size", 0, "eStargz chunk size")
	cmd.Flags().Int("estargz-min-chunk-size", 0, "The minimal number of bytes of data must be written in one gzip stream")
	cmd.Flags().Bool("zstdchunked", false, "Convert the committed layer to zstd:chunked for lazy pulling")
	cmd.Flags().Int("zstdchunked-compression-level", 3, "zstd:chunked compression level")
	cmd.Flags().Int("zstdchunked-chunk-size", 0, "zstd:chunked chunk size")
	cmd.Flags().Bool("devbox-remove-layer", false, "Remove the top layer of the base image when committing a devbox container")
	return cmd
}

func commitOptions(cmd *cobra.Command) (types.ContainerCommitOptions, error) {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	author, err := cmd.Flags().GetString("author")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	message, err := cmd.Flags().GetString("message")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	pause, err := cmd.Flags().GetBool("pause")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	change, err := cmd.Flags().GetStringArray("change")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	com, err := cmd.Flags().GetString("compression")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	if com != string(types.Zstd) && com != string(types.Gzip) {
		return types.ContainerCommitOptions{}, errors.New("--compression param only supports zstd or gzip")
	}

	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	if format != string(types.ImageFormatDocker) && format != string(types.ImageFormatOCI) {
		return types.ContainerCommitOptions{}, errors.New("--format param only supports docker or oci")
	}

	estargz, err := cmd.Flags().GetBool("estargz")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	estargzCompressionLevel, err := cmd.Flags().GetInt("estargz-compression-level")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	estargzChunkSize, err := cmd.Flags().GetInt("estargz-chunk-size")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	estargzMinChunkSize, err := cmd.Flags().GetInt("estargz-min-chunk-size")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	zstdchunked, err := cmd.Flags().GetBool("zstdchunked")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	zstdchunkedCompressionLevel, err := cmd.Flags().GetInt("zstdchunked-compression-level")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}
	zstdchunkedChunkSize, err := cmd.Flags().GetInt("zstdchunked-chunk-size")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	// estargz and zstdchunked are mutually exclusive
	if estargz && zstdchunked {
		return types.ContainerCommitOptions{}, errors.New("options --estargz and --zstdchunked lead to conflict, only one of them can be used")
	}

	removeBaseImageTopLayer, err := cmd.Flags().GetBool("devbox-remove-layer")
	if err != nil {
		return types.ContainerCommitOptions{}, err
	}

	return types.ContainerCommitOptions{
		Stdout:      cmd.OutOrStdout(),
		GOptions:    globalOptions,
		Author:      author,
		Message:     message,
		Pause:       pause,
		Change:      change,
		Compression: types.CompressionType(com),
		Format:      types.ImageFormat(format),
		EstargzOptions: types.EstargzOptions{
			Estargz:                 estargz,
			EstargzCompressionLevel: estargzCompressionLevel,
			EstargzChunkSize:        estargzChunkSize,
			EstargzMinChunkSize:     estargzMinChunkSize,
		},
		ZstdChunkedOptions: types.ZstdChunkedOptions{
			ZstdChunked:                 zstdchunked,
			ZstdChunkedCompressionLevel: zstdchunkedCompressionLevel,
			ZstdChunkedChunkSize:        zstdchunkedChunkSize,
		},
		DevboxOptions: types.DevboxOptions{
			RemoveBaseImageTopLayer: removeBaseImageTopLayer,
		},
	}, nil
}

func commitAction(cmd *cobra.Command, args []string) error {
	options, err := commitOptions(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), options.GOptions.Namespace, options.GOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	return container.Commit(ctx, client, args[1], args[0], options)
}

func commitShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) == 0 {
		return completion.ContainerNames(cmd, nil)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}
