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
	"os"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/namespaces"
	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/version"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Category = string

const (
	CategoryManagement = Category("Management")
)

func main() {
	if err := xmain(); err != nil {
		HandleExitCoder(err)
		logrus.Fatal(err)
	}
}

func xmain() error {
	if len(os.Args) == 3 && os.Args[1] == logging.MagicArgv1 {
		// containerd runtime v2 logging plugin mode.
		// "binary://BIN?KEY=VALUE" URI is parsed into Args {BIN, KEY, VALUE}.
		return logging.Main(os.Args[2])
	}
	// nerdctl CLI mode
	return newApp().Execute()
}

func newApp() *cobra.Command {
	var rootCmd = &cobra.Command{
		Use:           "nerdctl",
		Short:         "nerdctl is a command line interface for containerd",
		Version:       strings.TrimPrefix(version.Version, "v"),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.PersistentFlags().Bool("debug", false, "debug mode")
	rootCmd.PersistentFlags().Bool("debug-full", false, "debug mode (with full output)")
	{
		address := new(string)
		rootCmd.PersistentFlags().AddFlag(
			&pflag.Flag{
				Name:      "address",
				Shorthand: "a",
				Usage:     `containerd address, optionally with "unix://" prefix`,
				EnvVars:   []string{"CONTAINERD_ADDRESS"},
				Value:     pflag.NewStringValue(defaults.DefaultAddress, address),
			},
		)
		rootCmd.PersistentFlags().AddFlag(
			&pflag.Flag{
				Name:      "host",
				Shorthand: "H",
				Usage:     `alias of --address`,
				Value:     pflag.NewStringValue(defaults.DefaultAddress, address),
			},
		)
	}
	rootCmd.PersistentFlags().AddFlag(
		&pflag.Flag{
			Name:      "namespace",
			Shorthand: "n",
			Usage:     `containerd namespace, such as "moby" for Docker, "k8s.io" for Kubernetes`,
			EnvVars:   []string{"CONTAINERD_NAMESPACE"},
			Value:     pflag.NewStringValue(namespaces.Default, new(string)),
		},
	)
	rootCmd.RegisterFlagCompletionFunc("namespace", shellCompleteNamespaceNames)
	{
		snapshotter := new(string)
		rootCmd.PersistentFlags().AddFlag(
			&pflag.Flag{
				Name:    "snapshotter",
				Usage:   "containerd snapshotter",
				EnvVars: []string{"CONTAINERD_SNAPSHOTTER"},
				Value:   pflag.NewStringValue(containerd.DefaultSnapshotter, snapshotter),
			},
		)
		rootCmd.PersistentFlags().AddFlag(
			&pflag.Flag{
				Name:  "storage-driver",
				Usage: "alias of --snapshotter",
				Value: pflag.NewStringValue(containerd.DefaultSnapshotter, snapshotter),
			},
		)
		rootCmd.RegisterFlagCompletionFunc("snapshotter", shellCompleteSnapshotterNames)
		rootCmd.RegisterFlagCompletionFunc("storage-driver", shellCompleteSnapshotterNames)
	}

	rootCmd.PersistentFlags().AddFlag(
		&pflag.Flag{
			Name:    "cni-path",
			Usage:   "Set the cni-plugins binary directory",
			EnvVars: []string{"CNI_PATH"},
			Value:   pflag.NewStringValue(ncdefaults.CNIPath(), new(string)),
		},
	)
	rootCmd.PersistentFlags().AddFlag(
		&pflag.Flag{
			Name:    "cni-netconfpath",
			Usage:   "Set the CNI config directory",
			EnvVars: []string{"NETCONFPATH"},
			Value:   pflag.NewStringValue(ncdefaults.CNINetConfPath(), new(string)),
		},
	)
	rootCmd.PersistentFlags().String("data-root", ncdefaults.DataRoot(), "Root directory of persistent nerdctl state (managed by nerdctl, not by containerd)")
	rootCmd.PersistentFlags().String("cgroup-manager", ncdefaults.CgroupManager(), `Cgroup manager to use ("cgroupfs"|"systemd")`)
	rootCmd.RegisterFlagCompletionFunc("cgroup-manager", shellCompleteCgroupManagerNames)
	rootCmd.PersistentFlags().Bool("insecure-registry", false, "skips verifying HTTPS certs, and allows falling back to plain HTTP")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		debug, err := cmd.Flags().GetBool("debug-full")
		if err != nil {
			return err
		}
		if !debug {
			debug, err = cmd.Flags().GetBool("debug")
			if err != nil {
				return err
			}
		}
		if debug {
			logrus.SetLevel(logrus.DebugLevel)
		}
		address := cmd.Flags().Lookup("address").Value.String()
		if strings.Contains(address, "://") && !strings.HasPrefix(address, "unix://") {
			return fmt.Errorf("invalid address %q", address)
		}
		cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
		if err != nil {
			return err
		}
		if runtime.GOOS == "linux" {
			switch cgroupManager {
			case "systemd", "cgroupfs", "none":
			default:
				return fmt.Errorf("invalid cgroup-manager %q (supported values: \"systemd\", \"cgroupfs\", \"none\")", cgroupManager)
			}
		}
		if appNeedsRootlessParentMain(cmd, args) {
			// reexec /proc/self/exe with `nsenter` into RootlessKit namespaces
			return rootlessutil.ParentMain()
		}
		return nil
	}
	rootCmd.AddCommand(
		// #region Run & Exec
		newRunCommand(),
		newExecCommand(),
		// #endregion

		// #region Container management
		newPsCommand(),
		newLogsCommand(),
		newPortCommand(),
		newStopCommand(),
		newStartCommand(),
		newRestartCommand(),
		newKillCommand(),
		newRmCommand(),
		newPauseCommand(),
		newUnpauseCommand(),
		newCommitCommand(),
		newWaitCommand(),
		// #endregion

		// Build
		newBuildCommand(),

		// #region Image management
		newImagesCommand(),
		newPullCommand(),
		newPushCommand(),
		newLoadCommand(),
		newSaveCommand(),
		newTagCommand(),
		newRmiCommand(),
		// #endregion

		// #region System
		newEventsCommand(),
		newInfoCommand(),
		newVersionCommand(),
		// #endregion

		// Inspect
		newInspectCommand(),

		// stats
		newTopCommand(),
		newStatsCommand(),

		// #region Management
		newContainerCommand(),
		newImageCommand(),
		newNetworkCommand(),
		newVolumeCommand(),
		newSystemCommand(),
		newNamespaceCommand(),
		// #endregion

		// Internal
		newInternalCommand(),

		// login
		newLoginCommand(),

		// Logout
		newLogoutCommand(),

		// Compose
		newComposeCommand(),
	)
	return rootCmd
}

func globalFlags(cmd *cobra.Command) (string, []string) {
	args0, err := os.Executable()
	if err != nil {
		logrus.WithError(err).Warnf("cannot call os.Executable(), assuming the executable to be %q", os.Args[0])
		args0 = os.Args[0]
	}
	if len(os.Args) < 2 {
		return args0, nil
	}

	rootCmd := cmd.Root()
	flagSet := rootCmd.PersistentFlags()
	args := []string{}
	flagSet.VisitAll(func(f *pflag.Flag) {
		key := f.Name
		val := f.Value.String()
		if f.Changed {
			args = append(args, "--"+key+"="+val)
		}
	})
	return args0, args
}

type ExitCoder interface {
	error
	ExitCode() int
}

type ExitCodeError struct {
	error
	exitCode int
}

func (e ExitCodeError) ExitCode() int {
	return e.exitCode
}

func HandleExitCoder(err error) {
	if err == nil {
		return
	}

	if exitErr, ok := err.(ExitCoder); ok {
		os.Exit(exitErr.ExitCode())
	}
}

// unknownSubcommandAction is needed to let `nerdctl system non-existent-command` fail
// https://github.com/containerd/nerdctl/issues/487
//
// Ideally this should be implemented in Cobra itself.
func unknownSubcommandAction(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return cmd.Help()
	}
	// The output mimics https://github.com/spf13/cobra/blob/v1.2.1/command.go#L647-L662
	msg := fmt.Sprintf("unknown subcommand %q for %q", args[0], cmd.Name())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, s := range suggestions {
			msg += fmt.Sprintf("\t%v\n", s)
		}
	}
	return errors.New(msg)
}
