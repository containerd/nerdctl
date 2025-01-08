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

	"github.com/fatih/color"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/builder"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/compose"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/container"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/image"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/inspect"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/internal"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/ipfs"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/login"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/namespace"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/network"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/system"
	"github.com/containerd/nerdctl/v2/cmd/nerdctl/volume"
	"github.com/containerd/nerdctl/v2/pkg/config"
	ncdefaults "github.com/containerd/nerdctl/v2/pkg/defaults"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/logging"
	"github.com/containerd/nerdctl/v2/pkg/rootlessutil"
	"github.com/containerd/nerdctl/v2/pkg/store"
	"github.com/containerd/nerdctl/v2/pkg/version"
)

var (
	// To print Bold Text
	Bold = color.New(color.Bold).SprintfFunc()
)

// usage was derived from https://github.com/spf13/cobra/blob/v1.2.1/command.go#L491-L514
func usage(c *cobra.Command) error {
	s := "Usage: "
	if c.Runnable() {
		s += c.UseLine() + "\n"
	} else {
		s += c.CommandPath() + " [command]\n"
	}
	s += "\n"
	if len(c.Aliases) > 0 {
		s += "Aliases: " + c.NameAndAliases() + "\n"
	}
	if c.HasExample() {
		s += "Example:\n"
		s += c.Example + "\n"
	}

	var managementCommands, nonManagementCommands []*cobra.Command
	for _, f := range c.Commands() {
		f := f
		if f.Hidden {
			continue
		}
		if f.Annotations[helpers.Category] == helpers.Management {
			managementCommands = append(managementCommands, f)
		} else {
			nonManagementCommands = append(nonManagementCommands, f)
		}
	}
	printCommands := func(title string, commands []*cobra.Command) string {
		if len(commands) == 0 {
			return ""
		}
		var longest int
		for _, f := range commands {
			if l := len(f.Name()); l > longest {
				longest = l
			}
		}

		title = Bold(title)
		t := title + ":\n"
		for _, f := range commands {
			t += "  "
			t += f.Name()
			t += strings.Repeat(" ", longest-len(f.Name()))
			t += "  " + f.Short + "\n"
		}
		t += "\n"
		return t
	}
	s += printCommands("helpers.Management commands", managementCommands)
	s += printCommands("Commands", nonManagementCommands)

	s += Bold("Flags") + ":\n"
	s += c.LocalFlags().FlagUsages() + "\n"

	if c == c.Root() {
		s += "Run '" + c.CommandPath() + " COMMAND --help' for more information on a command.\n"
	} else {
		s += "See also '" + c.Root().CommandPath() + " --help' for the global flags such as '--namespace', '--snapshotter', and '--cgroup-manager'."
	}
	fmt.Fprintln(c.OutOrStdout(), s)
	return nil
}

func main() {
	if err := xmain(); err != nil {
		errutil.HandleExitCoder(err)
		log.L.Fatal(err)
	}
}

func xmain() error {
	if len(os.Args) == 3 && os.Args[1] == logging.MagicArgv1 {
		// containerd runtime v2 logging plugin mode.
		// "binary://BIN?KEY=VALUE" URI is parsed into Args {BIN, KEY, VALUE}.
		return logging.Main(os.Args[2])
	}
	// nerdctl CLI mode
	app, err := newApp()
	if err != nil {
		return err
	}
	return app.Execute()
}

func initRootCmdFlags(rootCmd *cobra.Command, tomlPath string) (*pflag.FlagSet, error) {
	cfg := config.New()
	if r, err := os.Open(tomlPath); err == nil {
		log.L.Debugf("Loading config from %q", tomlPath)
		defer r.Close()
		dec := toml.NewDecoder(r).DisallowUnknownFields() // set Strict to detect typo
		if err := dec.Decode(cfg); err != nil {
			return nil, fmt.Errorf("failed to load nerdctl config (not daemon config) from %q (Hint: don't mix up daemon's `config.toml` with `nerdctl.toml`): %w", tomlPath, err)
		}
		log.L.Debugf("Loaded config %+v", cfg)
	} else {
		log.L.WithError(err).Debugf("Not loading config from %q", tomlPath)
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	aliasToBeInherited := pflag.NewFlagSet(rootCmd.Name(), pflag.ExitOnError)

	rootCmd.PersistentFlags().Bool("debug", cfg.Debug, "debug mode")
	rootCmd.PersistentFlags().Bool("debug-full", cfg.DebugFull, "debug mode (with full output)")
	// -a is aliases (conflicts with nerdctl images -a)
	helpers.AddPersistentStringFlag(rootCmd, "address", []string{"a", "H"}, nil, []string{"host"}, aliasToBeInherited, cfg.Address, "CONTAINERD_ADDRESS", `containerd address, optionally with "unix://" prefix`)
	// -n is aliases (conflicts with nerdctl logs -n)
	helpers.AddPersistentStringFlag(rootCmd, "namespace", []string{"n"}, nil, nil, aliasToBeInherited, cfg.Namespace, "CONTAINERD_NAMESPACE", `containerd namespace, such as "moby" for Docker, "k8s.io" for Kubernetes`)
	rootCmd.RegisterFlagCompletionFunc("namespace", completion.NamespaceNames)
	helpers.AddPersistentStringFlag(rootCmd, "snapshotter", nil, nil, []string{"storage-driver"}, aliasToBeInherited, cfg.Snapshotter, "CONTAINERD_SNAPSHOTTER", "containerd snapshotter")
	rootCmd.RegisterFlagCompletionFunc("snapshotter", completion.SnapshotterNames)
	rootCmd.RegisterFlagCompletionFunc("storage-driver", completion.SnapshotterNames)
	helpers.AddPersistentStringFlag(rootCmd, "cni-path", nil, nil, nil, aliasToBeInherited, cfg.CNIPath, "CNI_PATH", "cni plugins binary directory")
	helpers.AddPersistentStringFlag(rootCmd, "cni-netconfpath", nil, nil, nil, aliasToBeInherited, cfg.CNINetConfPath, "NETCONFPATH", "cni config directory")
	rootCmd.PersistentFlags().String("data-root", cfg.DataRoot, "Root directory of persistent nerdctl state (managed by nerdctl, not by containerd)")
	rootCmd.PersistentFlags().String("cgroup-manager", cfg.CgroupManager, `Cgroup manager to use ("cgroupfs"|"systemd")`)
	rootCmd.RegisterFlagCompletionFunc("cgroup-manager", completion.CgroupManagerNames)
	rootCmd.PersistentFlags().Bool("insecure-registry", cfg.InsecureRegistry, "skips verifying HTTPS certs, and allows falling back to plain HTTP")
	// hosts-dir is defined as StringSlice, not StringArray, to allow specifying "--hosts-dir=/etc/containerd/certs.d,/etc/docker/certs.d"
	rootCmd.PersistentFlags().StringSlice("hosts-dir", cfg.HostsDir, "A directory that contains <HOST:PORT>/hosts.toml (containerd style) or <HOST:PORT>/{ca.cert, cert.pem, key.pem} (docker style)")
	// Experimental enable experimental feature, see in https://github.com/containerd/nerdctl/blob/main/docs/experimental.md
	helpers.AddPersistentBoolFlag(rootCmd, "experimental", nil, nil, cfg.Experimental, "NERDCTL_EXPERIMENTAL", "Control experimental: https://github.com/containerd/nerdctl/blob/main/docs/experimental.md")
	helpers.AddPersistentStringFlag(rootCmd, "host-gateway-ip", nil, nil, nil, aliasToBeInherited, cfg.HostGatewayIP, "NERDCTL_HOST_GATEWAY_IP", "IP address that the special 'host-gateway' string in --add-host resolves to. Defaults to the IP address of the host. It has no effect without setting --add-host")
	helpers.AddPersistentStringFlag(rootCmd, "bridge-ip", nil, nil, nil, aliasToBeInherited, cfg.BridgeIP, "NERDCTL_BRIDGE_IP", "IP address for the default nerdctl bridge network")
	rootCmd.PersistentFlags().Bool("kube-hide-dupe", cfg.KubeHideDupe, "Deduplicate images for Kubernetes with namespace k8s.io")
	return aliasToBeInherited, nil
}

func newApp() (*cobra.Command, error) {

	tomlPath := ncdefaults.NerdctlTOML()
	if v, ok := os.LookupEnv("NERDCTL_TOML"); ok {
		tomlPath = v
	}

	short := "nerdctl is a command line interface for containerd"
	long := fmt.Sprintf(`%s

Config file ($NERDCTL_TOML): %s
`, short, tomlPath)
	var rootCmd = &cobra.Command{
		Use:              "nerdctl",
		Short:            short,
		Long:             long,
		Version:          strings.TrimPrefix(version.GetVersion(), "v"),
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true, // required for global short hands like -a, -H, -n
	}

	rootCmd.SetUsageFunc(usage)
	aliasToBeInherited, err := initRootCmdFlags(rootCmd, tomlPath)
	if err != nil {
		return nil, err
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
		if err != nil {
			return err
		}
		debug := globalOptions.DebugFull
		if !debug {
			debug = globalOptions.Debug
		}
		if debug {
			log.SetLevel(log.DebugLevel.String())
		}
		address := globalOptions.Address
		if strings.Contains(address, "://") && !strings.HasPrefix(address, "unix://") {
			return fmt.Errorf("invalid address %q", address)
		}
		cgroupManager := globalOptions.CgroupManager
		if runtime.GOOS == "linux" {
			switch cgroupManager {
			case "systemd", "cgroupfs", "none":
			default:
				return fmt.Errorf("invalid cgroup-manager %q (supported values: \"systemd\", \"cgroupfs\", \"none\")", cgroupManager)
			}
		}

		// Since we store containers' stateful information on the filesystem per namespace, we need namespaces to be
		// valid, safe path segments. This is enforced by store.ValidatePathComponent.
		// Note that the container runtime will further enforce additional restrictions on namespace names
		// (containerd treats namespaces as valid identifiers - eg: alphanumericals + dash, starting with a letter)
		// See https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#path-segment-names for
		// considerations about path segments identifiers.
		if err = store.ValidatePathComponent(globalOptions.Namespace); err != nil {
			return err
		}
		if appNeedsRootlessParentMain(cmd, args) {
			// reexec /proc/self/exe with `nsenter` into RootlessKit namespaces
			return rootlessutil.ParentMain(globalOptions.HostGatewayIP)
		}
		return nil
	}
	rootCmd.RunE = helpers.UnknownSubcommandAction
	rootCmd.AddCommand(
		container.NewCreateCommand(),
		// #region Run & Exec
		container.NewRunCommand(),
		container.NewUpdateCommand(),
		container.NewExecCommand(),
		// #endregion

		// #region Container management
		container.NewPsCommand(),
		container.NewLogsCommand(),
		container.NewPortCommand(),
		container.NewStopCommand(),
		container.NewStartCommand(),
		container.NewDiffCommand(),
		container.NewRestartCommand(),
		container.NewKillCommand(),
		container.NewRmCommand(),
		container.NewPauseCommand(),
		container.NewUnpauseCommand(),
		container.NewCommitCommand(),
		container.NewWaitCommand(),
		container.NewRenameCommand(),
		container.NewAttachCommand(),
		// #endregion

		// Build
		builder.NewBuildCommand(),

		// #region Image management
		image.NewImagesCommand(),
		image.NewPullCommand(),
		image.NewPushCommand(),
		image.NewLoadCommand(),
		image.NewSaveCommand(),
		image.NewTagCommand(),
		image.NewRmiCommand(),
		image.NewHistoryCommand(),
		// #endregion

		// #region System
		system.NewEventsCommand(),
		system.NewInfoCommand(),
		newVersionCommand(),
		// #endregion

		// Inspect
		inspect.NewInspectCommand(),

		// stats
		container.NewTopCommand(),
		container.NewStatsCommand(),

		// #region helpers.Management
		container.NewContainerCommand(),
		image.NewImageCommand(),
		network.NewNetworkCommand(),
		volume.NewVolumeCommand(),
		system.NewSystemCommand(),
		namespace.NewNamespaceCommand(),
		builder.NewBuilderCommand(),
		// #endregion

		// Internal
		internal.NewInternalCommand(),

		// login
		login.NewLoginCommand(),

		// Logout
		login.NewLogoutCommand(),

		// Compose
		compose.NewComposeCommand(),

		// IPFS
		ipfs.NewIPFSCommand(),
	)
	addApparmorCommand(rootCmd)
	container.AddCpCommand(rootCmd)

	// add aliasToBeInherited to subCommand(s) InheritedFlags
	for _, subCmd := range rootCmd.Commands() {
		subCmd.InheritedFlags().AddFlagSet(aliasToBeInherited)
	}
	return rootCmd, nil
}
