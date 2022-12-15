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
	"github.com/containerd/nerdctl/cmd/nerdctl/apparmor"
	"github.com/containerd/nerdctl/cmd/nerdctl/builder"
	"github.com/containerd/nerdctl/cmd/nerdctl/completion"
	"github.com/containerd/nerdctl/cmd/nerdctl/compose"
	"github.com/containerd/nerdctl/cmd/nerdctl/container"
	"github.com/containerd/nerdctl/cmd/nerdctl/image"
	"github.com/containerd/nerdctl/cmd/nerdctl/inspect"
	"github.com/containerd/nerdctl/cmd/nerdctl/internal"
	"github.com/containerd/nerdctl/cmd/nerdctl/ipfs"
	"github.com/containerd/nerdctl/cmd/nerdctl/login"
	"github.com/containerd/nerdctl/cmd/nerdctl/logout"
	"github.com/containerd/nerdctl/cmd/nerdctl/namespace"
	"github.com/containerd/nerdctl/cmd/nerdctl/network"
	"github.com/containerd/nerdctl/cmd/nerdctl/system"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/common"
	version2 "github.com/containerd/nerdctl/cmd/nerdctl/version"
	"github.com/containerd/nerdctl/cmd/nerdctl/volume"
	ncdefaults "github.com/containerd/nerdctl/pkg/defaults"
	"github.com/containerd/nerdctl/pkg/logging"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/version"
	"github.com/pelletier/go-toml"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		if f.Annotations[common.Category] == common.Management {
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
	s += printCommands("Management commands", managementCommands)
	s += printCommands("Commands", nonManagementCommands)

	s += "Flags:\n"
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
		common.HandleExitCoder(err)
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
	app, err := newApp()
	if err != nil {
		return err
	}
	return app.Execute()
}

// Config corresponds to nerdctl.toml .
// See docs/config.md .
type Config struct {
	Debug            bool     `toml:"debug"`
	DebugFull        bool     `toml:"debug_full"`
	Address          string   `toml:"address"`
	Namespace        string   `toml:"namespace"`
	Snapshotter      string   `toml:"snapshotter"`
	CNIPath          string   `toml:"cni_path"`
	CNINetConfPath   string   `toml:"cni_netconfpath"`
	DataRoot         string   `toml:"data_root"`
	CgroupManager    string   `toml:"cgroup_manager"`
	InsecureRegistry bool     `toml:"insecure_registry"`
	HostsDir         []string `toml:"hosts_dir"`
	Experimental     bool     `toml:"experimental"`
}

// NewConfig creates a default Config object statically,
// without interpolating CLI flags, env vars, and toml.
func NewConfig() *Config {
	return &Config{
		Debug:            false,
		DebugFull:        false,
		Address:          defaults.DefaultAddress,
		Namespace:        namespaces.Default,
		Snapshotter:      containerd.DefaultSnapshotter,
		CNIPath:          ncdefaults.CNIPath(),
		CNINetConfPath:   ncdefaults.CNINetConfPath(),
		DataRoot:         ncdefaults.DataRoot(),
		CgroupManager:    ncdefaults.CgroupManager(),
		InsecureRegistry: false,
		HostsDir:         ncdefaults.HostsDirs(),
		Experimental:     true,
	}
}

func initRootCmdFlags(rootCmd *cobra.Command, tomlPath string) (*pflag.FlagSet, error) {
	cfg := NewConfig()
	if r, err := os.Open(tomlPath); err == nil {
		logrus.Debugf("Loading config from %q", tomlPath)
		defer r.Close()
		dec := toml.NewDecoder(r).Strict(true) // set Strict to detect typo
		if err := dec.Decode(cfg); err != nil {
			return nil, fmt.Errorf("failed to load nerdctl config (not daemon config) from %q (Hint: don't mix up daemon's `config.toml` with `nerdctl.toml`): %w", tomlPath, err)
		}
		logrus.Debugf("Loaded config %+v", cfg)
	} else {
		logrus.WithError(err).Debugf("Not loading config from %q", tomlPath)
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	aliasToBeInherited := pflag.NewFlagSet(rootCmd.Name(), pflag.ExitOnError)

	rootCmd.PersistentFlags().Bool("debug", cfg.Debug, "debug mode")
	rootCmd.PersistentFlags().Bool("debug-full", cfg.DebugFull, "debug mode (with full output)")
	// -a is aliases (conflicts with nerdctl images -a)
	utils.AddPersistentStringFlag(rootCmd, "address", []string{"a", "H"}, nil, []string{"host"}, aliasToBeInherited, cfg.Address, "CONTAINERD_ADDRESS", `containerd address, optionally with "unix://" prefix`)
	// -n is aliases (conflicts with nerdctl logs -n)
	utils.AddPersistentStringFlag(rootCmd, "namespace", []string{"n"}, nil, nil, aliasToBeInherited, cfg.Namespace, "CONTAINERD_NAMESPACE", `containerd namespace, such as "moby" for Docker, "k8s.io" for Kubernetes`)
	rootCmd.RegisterFlagCompletionFunc("namespace", completion.ShellCompleteNamespaceNames)
	utils.AddPersistentStringFlag(rootCmd, "snapshotter", nil, nil, []string{"storage-driver"}, aliasToBeInherited, cfg.Snapshotter, "CONTAINERD_SNAPSHOTTER", "containerd snapshotter")
	rootCmd.RegisterFlagCompletionFunc("snapshotter", completion.ShellCompleteSnapshotterNames)
	rootCmd.RegisterFlagCompletionFunc("storage-driver", completion.ShellCompleteSnapshotterNames)
	utils.AddPersistentStringFlag(rootCmd, "cni-path", nil, nil, nil, aliasToBeInherited, cfg.CNIPath, "CNI_PATH", "cni plugins binary directory")
	utils.AddPersistentStringFlag(rootCmd, "cni-netconfpath", nil, nil, nil, aliasToBeInherited, cfg.CNINetConfPath, "NETCONFPATH", "cni config directory")
	rootCmd.PersistentFlags().String("data-root", cfg.DataRoot, "Root directory of persistent nerdctl state (managed by nerdctl, not by containerd)")
	rootCmd.PersistentFlags().String("cgroup-manager", cfg.CgroupManager, `Cgroup manager to use ("cgroupfs"|"systemd")`)
	rootCmd.RegisterFlagCompletionFunc("cgroup-manager", completion.ShellCompleteCgroupManagerNames)
	rootCmd.PersistentFlags().Bool("insecure-registry", cfg.InsecureRegistry, "skips verifying HTTPS certs, and allows falling back to plain HTTP")
	// hosts-dir is defined as StringSlice, not StringArray, to allow specifying "--hosts-dir=/etc/containerd/certs.d,/etc/docker/certs.d"
	rootCmd.PersistentFlags().StringSlice("hosts-dir", cfg.HostsDir, "A directory that contains <HOST:PORT>/hosts.toml (containerd style) or <HOST:PORT>/{ca.cert, cert.pem, key.pem} (docker style)")
	// Experimental enable experimental feature, see in https://github.com/containerd/nerdctl/blob/main/docs/experimental.md
	utils.AddPersistentBoolFlag(rootCmd, "experimental", nil, nil, cfg.Experimental, "NERDCTL_EXPERIMENTAL", "Control experimental: https://github.com/containerd/nerdctl/blob/main/docs/experimental.md")
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
		Version:          strings.TrimPrefix(version.Version, "v"),
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
	rootCmd.RunE = completion.UnknownSubcommandAction
	rootCmd.AddCommand(
		container.NewCreateCommand(),
		// #region Run & Exec
		container.NewRunCommand(),
		container.NewUpdateCommand(),
		container.NewExecCommand(),
		// #endregion

		// #region Container management
		container.NewPsCommandForMain(),
		container.NewLogsCommand(),
		container.NewPortCommand(),
		container.NewStopCommand(),
		container.NewStartCommand(),
		container.NewRestartCommand(),
		container.NewKillCommand(),
		container.NewRmCommand(),
		container.NewPauseCommand(),
		container.NewUnpauseCommand(),
		container.NewCommitCommand(),
		container.NewWaitCommand(),
		container.NewRenameCommand(),
		// #endregion

		// Build
		builder.NewBuildCommand(),

		// #region Image management
		image.NewImagesCommandForMain(),
		image.NewPullCommand(),
		image.NewPushCommand(),
		image.NewLoadCommand(),
		image.NewSaveCommand(),
		image.NewTagCommand(),
		image.NewRmiCommandForMain(),
		image.NewHistoryCommand(),
		// #endregion

		// #region System
		system.NewEventsCommand(),
		system.NewInfoCommand(),
		version2.NewVersionCommand(),
		// #endregion

		// Inspect
		inspect.NewInspectCommand(),

		// stats
		container.NewTopCommand(),
		container.NewStatsCommand(),

		// #region Management
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
		logout.NewLogoutCommand(),

		// Compose
		compose.NewComposeCommand(),

		// IPFS
		ipfs.NewIPFSCommand(),
	)
	apparmor.AddApparmorCommand(rootCmd)
	container.AddCpCommand(rootCmd)

	// add aliasToBeInherited to subCommand(s) InheritedFlags
	for _, subCmd := range rootCmd.Commands() {
		subCmd.InheritedFlags().AddFlagSet(aliasToBeInherited)
	}
	return rootCmd, nil
}
