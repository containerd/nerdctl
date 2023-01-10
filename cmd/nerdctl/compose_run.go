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
	"errors"
	"fmt"

	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/cmd/compose"
	"github.com/containerd/nerdctl/pkg/composer"
	"github.com/spf13/cobra"
)

func newComposeRunCommand() *cobra.Command {
	var composeRunCommand = &cobra.Command{
		Use:                   "run [flags] SERVICE [COMMAND] [ARGS...]",
		Short:                 "Run a one-off command on a service",
		Args:                  cobra.MinimumNArgs(1),
		RunE:                  composeRunAction,
		SilenceUsage:          true,
		SilenceErrors:         true,
		DisableFlagsInUseLine: true,
	}
	composeRunCommand.Flags().SetInterspersed(false)
	composeRunCommand.Flags().BoolP("detach", "d", false, "Detached mode: Run containers in the background")
	composeRunCommand.Flags().Bool("no-build", false, "Don't build an image, even if it's missing.")
	composeRunCommand.Flags().Bool("no-color", false, "Produce monochrome output")
	composeRunCommand.Flags().Bool("no-log-prefix", false, "Don't print prefix in logs")
	composeRunCommand.Flags().Bool("build", false, "Build images before starting containers.")
	composeRunCommand.Flags().Bool("quiet-pull", false, "Pull without printing progress information")
	composeRunCommand.Flags().Bool("remove-orphans", false, "Remove containers for services not defined in the Compose file.")

	composeRunCommand.Flags().String("name", "", "Assign a name to the container")
	composeRunCommand.Flags().Bool("no-deps", false, "Don't start dependencies")
	// TODO: no-TTY flag
	//       In docker-compose's documentation, no-TTY is automatically detected
	//       But, it follows `-i` flag because currently `run` command needs `-it` simultaneously.
	composeRunCommand.Flags().BoolP("interactive", "i", true, "Keep STDIN open even if not attached")
	composeRunCommand.Flags().Bool("rm", false, "Automatically remove the container when it exits")
	composeRunCommand.Flags().StringP("user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	composeRunCommand.Flags().StringArrayP("volume", "v", nil, "Bind mount a volume")
	composeRunCommand.Flags().StringArray("entrypoint", nil, "Overwrite the default ENTRYPOINT of the image")
	composeRunCommand.Flags().StringArrayP("env", "e", nil, "Set environment variables")
	composeRunCommand.Flags().StringArrayP("label", "l", nil, "Set metadata on container")
	composeRunCommand.Flags().StringP("workdir", "w", "", "Working directory inside the container")
	// FIXME: `-p` conflicts with the `--project-name` in PersistentFlags of parent command `compose`
	//        For docker compatibility, it should be fixed.
	composeRunCommand.Flags().StringSlice("publish", nil, "Publish a container's port(s) to the host")
	composeRunCommand.Flags().Bool("service-ports", false, "Run command with the service's ports enabled and mapped to the host")
	// TODO: use-aliases

	return composeRunCommand
}

func composeRunAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	detach, err := cmd.Flags().GetBool("detach")
	if err != nil {
		return err
	}
	noBuild, err := cmd.Flags().GetBool("no-build")
	if err != nil {
		return err
	}
	noColor, err := cmd.Flags().GetBool("no-color")
	if err != nil {
		return err
	}
	noLogPrefix, err := cmd.Flags().GetBool("no-log-prefix")
	if err != nil {
		return err
	}
	build, err := cmd.Flags().GetBool("build")
	if err != nil {
		return err
	}
	if build && noBuild {
		return errors.New("--build and --no-build can not be combined")
	}
	quietPull, err := cmd.Flags().GetBool("quiet-pull")
	if err != nil {
		return err
	}
	removeOrphans, err := cmd.Flags().GetBool("remove-orphans")
	if err != nil {
		return err
	}

	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	nodeps, err := cmd.Flags().GetBool("no-deps")
	if err != nil {
		return err
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}
	// FIXME : https://github.com/containerd/nerdctl/blob/v0.22.2/cmd/nerdctl/run.go#L100
	tty := interactive
	rm, err := cmd.Flags().GetBool("rm")
	if err != nil {
		return err
	}
	user, err := cmd.Flags().GetString("user")
	if err != nil {
		return err
	}
	volume, err := cmd.Flags().GetStringArray("volume")
	if err != nil {
		return err
	}
	entrypoint, err := cmd.Flags().GetStringArray("entrypoint")
	if err != nil {
		return err
	}
	env, err := cmd.Flags().GetStringArray("env")
	if err != nil {
		return err
	}
	label, err := cmd.Flags().GetStringArray("label")
	if err != nil {
		return err
	}
	workdir, err := cmd.Flags().GetString("workdir")
	if err != nil {
		return err
	}
	publish, err := cmd.Flags().GetStringSlice("publish")
	if err != nil {
		return err
	}
	servicePorts, err := cmd.Flags().GetBool("service-ports")
	if err != nil {
		return err
	}

	if servicePorts && publish != nil && len(publish) > 0 {
		return fmt.Errorf("--service-ports and --publish(-p) cannot exist simultaneously")
	}
	// https://github.com/containerd/nerdctl/blob/v0.22.2/cmd/nerdctl/run.go#L475
	if interactive && detach {
		return errors.New("currently flag -i and -d cannot be specified together (FIXME)")
	}

	// https://github.com/containerd/nerdctl/blob/v0.22.2/cmd/nerdctl/run.go#L479
	if tty && detach {
		return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
	}

	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	options, err := getComposeOptions(cmd, globalOptions.DebugFull, globalOptions.Experimental)
	if err != nil {
		return err
	}
	c, err := compose.New(client, globalOptions, options, cmd.OutOrStdout(), cmd.ErrOrStderr())
	if err != nil {
		return err
	}

	ro := composer.RunOptions{
		Detach:        detach,
		NoBuild:       noBuild,
		NoColor:       noColor,
		NoLogPrefix:   noLogPrefix,
		ForceBuild:    build,
		QuietPull:     quietPull,
		RemoveOrphans: removeOrphans,

		ServiceName: args[0],
		Args:        args[1:],

		Name:         name,
		NoDeps:       nodeps,
		Tty:          tty,
		Interactive:  interactive,
		Rm:           rm,
		User:         user,
		Volume:       volume,
		Entrypoint:   entrypoint,
		Env:          env,
		Label:        label,
		WorkDir:      workdir,
		ServicePorts: servicePorts,
		Publish:      publish,
	}

	return c.Run(ctx, ro)
}
