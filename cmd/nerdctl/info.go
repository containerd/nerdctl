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
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
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
	var (
		tmpl *template.Template
		err  error
	)
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	if format != "" {
		tmpl, err = parseTemplate(format)
		if err != nil {
			return err
		}
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	mode, err := cmd.Flags().GetString("mode")
	if err != nil {
		return err
	}
	switch mode {
	case "dockercompat", "native":
	default:
		return fmt.Errorf("unknown mode %q", mode)
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
		ci, err := nativeClientInfo(cmd)
		if err != nil {
			return err
		}
		infoNative = &native.Info{
			Client: ci,
			Daemon: di,
		}
	case "dockercompat":
		snapshotter, err := cmd.Flags().GetString("snapshotter")
		if err != nil {
			return err
		}
		cgroupManager, err := cmd.Flags().GetString("cgroup-manager")
		if err != nil {
			return err
		}
		infoCompat, err = infoutil.Info(ctx, client, snapshotter, cgroupManager)
		if err != nil {
			return err
		}
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
		b, err := json.MarshalIndent(infoNative, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
	case "dockercompat":
		return prettyPrintInfoDockerCompat(cmd, infoCompat)
	}
	return nil
}

func nativeClientInfo(cmd *cobra.Command) (*native.ClientInfo, error) {
	var (
		ci  native.ClientInfo
		err error
	)
	flags := cmd.Flags()
	ci.Namespace, err = flags.GetString("namespace")
	if err != nil {
		return nil, err
	}
	ci.Snapshotter, err = flags.GetString("snapshotter")
	if err != nil {
		return nil, err
	}
	ci.CgroupManager, err = flags.GetString("cgroup-manager")
	if err != nil {
		return nil, err
	}
	return &ci, nil
}

func prettyPrintInfoDockerCompat(cmd *cobra.Command, info *dockercompat.Info) error {
	w := cmd.OutOrStdout()
	namespace, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}
	debug, err := cmd.Flags().GetBool("debug")
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", namespace)
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
			fmt.Fprintf(w, "   %s: %s\n", strings.Title(k), v)
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
