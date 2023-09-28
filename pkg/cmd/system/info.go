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

package system

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/api/types"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/containerd/containerd/api/services/introspection/v1"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/infoutil"
	"github.com/containerd/nerdctl/pkg/inspecttypes/dockercompat"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/containerd/nerdctl/pkg/rootlessutil"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/docker/go-units"
	"github.com/sirupsen/logrus"
)

func Info(ctx context.Context, client *containerd.Client, options types.SystemInfoOptions) error {
	var (
		tmpl *template.Template
		err  error
	)
	if options.Format != "" {
		tmpl, err = formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
	}

	var (
		infoNative *native.Info
		infoCompat *dockercompat.Info
	)
	switch options.Mode {
	case "native":
		di, err := infoutil.NativeDaemonInfo(ctx, client)
		if err != nil {
			return err
		}
		infoNative, err = fulfillNativeInfo(di, options.GOptions)
		if err != nil {
			return err
		}
	case "dockercompat":
		infoCompat, err = infoutil.Info(ctx, client, options.GOptions.Snapshotter, options.GOptions.CgroupManager)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown mode %q", options.Mode)
	}

	if tmpl != nil {
		var x interface{} = infoNative
		if infoCompat != nil {
			x = infoCompat
		}
		w := options.Stdout
		if err := tmpl.Execute(w, x); err != nil {
			return err
		}
		_, err = fmt.Fprintln(w)
		return err
	}

	switch options.Mode {
	case "native":
		return prettyPrintInfoNative(options.Stdout, infoNative)
	case "dockercompat":
		return prettyPrintInfoDockerCompat(options.Stdout, options.Stderr, infoCompat, options.GOptions)
	}
	return nil
}

func fulfillNativeInfo(di *native.DaemonInfo, globalOptions types.GlobalCommandOptions) (*native.Info, error) {
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
	fmt.Fprintln(w, "containerd Plugins:")
	for _, f := range enabledPlugins {
		fmt.Fprintf(w, " - %s.%s\n", f.Type, f.ID)
	}
	fmt.Fprintf(w, "containerd Plugins (disabled):\n")
	for _, f := range disabledPlugins {
		fmt.Fprintf(w, " - %s.%s\n", f.Type, f.ID)
	}
	return nil
}

func prettyPrintInfoDockerCompat(stdout io.Writer, stderr io.Writer, info *dockercompat.Info, globalOptions types.GlobalCommandOptions) error {
	w := stdout
	debug := globalOptions.Debug
	fmt.Fprintf(w, "Client:\n")
	fmt.Fprintf(w, " Namespace:\t%s\n", globalOptions.Namespace)
	fmt.Fprintf(w, " Debug Mode:\t%v\n", debug)
	fmt.Fprintln(w)
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
		fmt.Fprintln(stderr, strings.Join(info.Warnings, "\n"))
	}
	return nil
}
