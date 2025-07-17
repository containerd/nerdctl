package container

import (
	"fmt"
	"os"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/cmd/container"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

func ExportCommand() *cobra.Command {
	var exportCommand = &cobra.Command{
		Use:               "export [OPTIONS] CONTAINER",
		Args:              cobra.ExactArgs(1),
		Short:             "Export a containers filesystem as a tar archive",
		Long:              "Export a containers filesystem as a tar archive",
		RunE:              exportAction,
		ValidArgsFunction: exportShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}
	exportCommand.Flags().StringP("output", "o", "", "Write to a file, instead of STDOUT")

	return exportCommand
}

func exportAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("requires at least 1 argument")
	}

	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return err
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	writer := cmd.OutOrStdout()
	if output != "" {
		f, err := os.OpenFile(output, os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		defer f.Close()
		writer = f
	} else {
		if isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("cowardly refusing to save to a terminal. Use the -o flag or redirect")
		}
	}

	options := types.ContainerExportOptions{
		Stdout:   writer,
		GOptions: globalOptions,
	}

	return container.Export(ctx, client, args[0], options)
}

func exportShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// show container names
	return completion.ContainerNames(cmd, nil)
}
