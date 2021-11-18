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

const (
	Category   = "category"
	Management = "management"
)

// mainHelpTemplate was derived from https://github.com/spf13/cobra/blob/v1.2.1/command.go#L491-L514
const mainHelpTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}
Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}
Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}
Management commands:{{range .Commands}}{{if (eq (index .Annotations "category") "management")}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Commands:{{range .Commands}}{{if (eq (index .Annotations "category") "")}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}
Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}
Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}
Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}
Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

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
		Use:              "nerdctl",
		Short:            "nerdctl is a command line interface for containerd",
		Version:          strings.TrimPrefix(version.Version, "v"),
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true, // required for global short hands like -a, -H, -n
	}
	rootCmd.SetHelpTemplate(mainHelpTemplate)
	rootCmd.PersistentFlags().Bool("debug", false, "debug mode")
	rootCmd.PersistentFlags().Bool("debug-full", false, "debug mode (with full output)")
	// -a is nonPersistentAlias (conflicts with nerdctl images -a)
	AddPersistentStringFlag(rootCmd, "address", []string{"host"}, []string{"a", "H"}, defaults.DefaultAddress, "CONTAINERD_ADDRESS", `containerd address, optionally with "unix://" prefix`)
	// -n is nonPersistentAlias (conflicts with nerdctl logs -n)
	AddPersistentStringFlag(rootCmd, "namespace", nil, []string{"n"}, namespaces.Default, "CONTAINERD_NAMESPACE", `containerd namespace, such as "moby" for Docker, "k8s.io" for Kubernetes`)
	rootCmd.RegisterFlagCompletionFunc("namespace", shellCompleteNamespaceNames)
	AddPersistentStringFlag(rootCmd, "snapshotter", []string{"storage-driver"}, nil, containerd.DefaultSnapshotter, "CONTAINERD_SNAPSHOTTER", "containerd snapshotter")
	rootCmd.RegisterFlagCompletionFunc("snapshotter", shellCompleteSnapshotterNames)
	rootCmd.RegisterFlagCompletionFunc("storage-driver", shellCompleteSnapshotterNames)
	AddPersistentStringFlag(rootCmd, "cni-path", nil, nil, ncdefaults.CNIPath(), "CNI_PATH", "cni plugins binary directory")
	AddPersistentStringFlag(rootCmd, "cni-netconfpath", nil, nil, ncdefaults.CNINetConfPath(), "NETCONFPATH", "cni config directory")
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
	rootCmd.RunE = unknownSubcommandAction
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

		// IPFS
		newIPFSCommand(),
	)
	addApparmorCommand(rootCmd)
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

// AddStringFlag is similar to cmd.Flags().String but supports aliases and env var
func AddStringFlag(cmd *cobra.Command, name string, aliases []string, value string, env, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		value = envV
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new(string)
	flags := cmd.Flags()
	flags.StringVar(p, name, value, usage)
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.StringVar(p, a, value, aliasesUsage)
		}
	}
}

// AddPersistentStringFlag is similar to AddStringFlag but persistent.
// See https://github.com/spf13/cobra/blob/master/user_guide.md#persistent-flags to learn what is "persistent".
func AddPersistentStringFlag(cmd *cobra.Command, name string, aliases, nonPersistentAliases []string, value string, env, usage string) {
	if env != "" {
		usage = fmt.Sprintf("%s [$%s]", usage, env)
	}
	if envV, ok := os.LookupEnv(env); ok {
		value = envV
	}
	aliasesUsage := fmt.Sprintf("Alias of --%s", name)
	p := new(string)
	flags := cmd.Flags()
	for _, a := range nonPersistentAliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			flags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			flags.StringVar(p, a, value, aliasesUsage)
		}
	}

	persistentFlags := cmd.PersistentFlags()
	persistentFlags.StringVar(p, name, value, usage)
	for _, a := range aliases {
		if len(a) == 1 {
			// pflag doesn't support short-only flags, so we have to register long one as well here
			persistentFlags.StringVarP(p, a, a, value, aliasesUsage)
		} else {
			persistentFlags.StringVar(p, a, value, aliasesUsage)
		}
	}
}
