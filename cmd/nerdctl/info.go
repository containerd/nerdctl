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
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/containerd/nerdctl/pkg/api/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/containerd/containerd/api/services/introspection/v1"
	"github.com/containerd/nerdctl/pkg/clientutil"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func newInfoCommand() *cobra.Command {
	var infoCommand = &cobra.Command{
		Use:           "info",
		Args:          cobra.NoArgs,
		Short:         "Display system-wide information",
		RunE:          infoAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	infoCommand.Flags().String("mode", "dockercompat", `Information mode, "dockercompat" for Docker-compatible output, "native" for containerd-native output`)
	infoCommand.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"dockercompat", "native"}, cobra.ShellCompDirectiveNoFileComp
	})
	infoCommand.Flags().StringP("format", "f", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	infoCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return infoCommand
}

func infoAction(cmd *cobra.Command, args []string) error {
	globalOptions, err := processRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	var (
		tmpl *template.Template
	)
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "" {
		tmpl, err = formatter.ParseTemplate(format)
		if err != nil {
			return err
		}
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}

	var (
		infoNative *native.Info
		infoCompat *dockercompat.Info
	)
	switch mode {
	case "native":
		di, err := infoutil.NativeDaemonInfo(ctx, client)
		if err != nil {
			return err
		}
		infoNative, err = fulfillNativeInfo(cmd, di, globalOptions)
		if err != nil {
			return err
		}
	case "dockercompat":
		infoCompat, err = infoutil.Info(ctx, client, globalOptions.Snapshotter, globalOptions.CgroupManager)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}

	if tmpl != nil {
		var x interface{} = infoNative
		if infoCompat != nil {
			x = infoCompat
		}
		w := cmd.OutOrStdout()
		if err := tmpl.Execute(w, x); err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "\n")
		return err
	}

	switch mode {
	case "native":
		return prettyPrintInfoNative(cmd.OutOrStdout(), infoNative)
	case "dockercompat":
		return prettyPrintInfoDockerCompat(cmd, infoCompat, globalOptions)
	}
	return nil
}

func fulfillNativeInfo(cmd *cobra.Command, di *native.DaemonInfo, globalOptions *types.GlobalCommandOptions) (*native.Info, error) {
	info := &native.Info{
		Daemon: di,
	}
	info.Namespace = globalOptions.Namespace
	info.Snapshotter = globalOptions.Snapshotter
	info.CgroupManager = globalOptions.CgroupManager
	info.Rootless = rootlessutil.IsRootless()
	return info, nil
}

func prettyPrintInfoNative(w io.Writer, info *native.Info) error {
	fmt.Fprintf(w, "Namespace:          %s\n", info.Namespace)
	fmt.Fprintf(w, "Snapshotter:        %s\n", info.Snapshotter)
	fmt.Fprintf(w, "Cgroup Manager:     %s\n", info.CgroupManager)
	fmt.Fprintf(w, "Rootless:           %v\n", info.Rootless)
	fmt.Fprintf(w, "containerd Version: %s (%s)\n", info.Daemon.Version.Version, info.Daemon.Version.Revision)
	fmt.Fprintf(w, "containerd UUID:    %s\n", info.Daemon.Server.UUID)
	var disabledPlugins, enabledPlugins []*introspection.Plugin
	for _, f := range info.Daemon.Plugins.Plugins {
		if f.InitErr == nil {
			enabledPlugins = append(enabledPlugins, f)
		} else {
			disabledPlugins = append(disabledPlugins, f)
		}
	}
	sorter := func(x []*introspection.Plugin) func(int, int) bool {
		return func(i, j int) bool {
			return x[i].Type+"."+x[j].ID < x[j].Type+"."+x[j].ID
		}
	}
	sort.Slice(enabledPlugins, sorter(enabledPlugins))
	sort.Slice(disabledPlugins, sorter(disabledPlugins))
	fmt.Fprintf(w, "containerd Plugins:\n")
	for _, f := range enabledPlugins {
		fmt.Fprintf(w, " - %s.%s\n", f.Type, f.ID)
	}
	fmt.Fprintf(w, "containerd Plugins (disabled):\n")
	for _, f := range disabledPlugins {
		fmt.Fprintf(w, " - %s.%s\n", f.Type, f.ID)
	}
	return nil
}

func prettyPrintInfoDockerCompat(cmd *cobra.Command, info *dockercompat.Info, globalOptions *types.GlobalCommandOptions) error {
	w := cmd.OutOrStdout()
	debug := globalOptions.Debug
	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", globalOptions.Namespace)
	fmt.Fprintf(w, " Debug Mode:\t%v\n", debug)
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Server:\n")
	fmt.Fprintf(w, " Server Version: %s\n", info.ServerVersion)
	// Storage Driver is not really Server concept for nerdctl, but mimics `docker info` output
	fmt.Fprintf(w, " Storage Driver: %s\n", info.Driver)
	fmt.Fprintf(w, " Logging Driver: %s\n", info.LoggingDriver)
	fmt.Fprintf(w, " Cgroup Driver: %s\n", info.CgroupDriver)
	fmt.Fprintf(w, " Cgroup Version: %s\n", info.CgroupVersion)
	fmt.Fprintf(w, " Plugins:\n")
	fmt.Fprintf(w, "  Log: %s\n", strings.Join(info.Plugins.Log, " "))
	fmt.Fprintf(w, "  Storage: %s\n", strings.Join(info.Plugins.Storage, " "))
	fmt.Fprintf(w, " Security Options:\n")
	for _, s := range info.SecurityOptions {
		m, err := strutil.ParseCSVMap(s)
		if err != nil {
			logrus.WithError(err).Warnf("unparsable security option %q", s)
			continue
		}
		name := m["name"]
		if name == "" {
			logrus.Warnf("unparsable security option %q", s)
			continue
		}
		fmt.Fprintf(w, "  %s\n", name)
		for k, v := range m {
			if k == "name" {
				continue
			}
			fmt.Fprintf(w, "   %s: %s\n", cases.Title(language.English).String(k), v)
		}
	}
	fmt.Fprintf(w, " Kernel Version: %s\n", info.KernelVersion)
	fmt.Fprintf(w, " Operating System: %s\n", info.OperatingSystem)
	fmt.Fprintf(w, " OSType: %s\n", info.OSType)
	fmt.Fprintf(w, " Architecture: %s\n", info.Architecture)
	fmt.Fprintf(w, " CPUs: %d\n", info.NCPU)
	fmt.Fprintf(w, " Total Memory: %s\n", units.BytesSize(float64(info.MemTotal)))
	fmt.Fprintf(w, " Name: %s\n", info.Name)
	fmt.Fprintf(w, " ID: %s\n", info.ID)

	fmt.Fprintln(w)
	if len(info.Warnings) > 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), strings.Join(info.Warnings, "\n"))
	}
	return nil
}
