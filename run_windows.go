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
	"context"
	winopts "github.com/Microsoft/hcsshim/cmd/containerd-shim-runhcs-v1/options"
	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

// modified from https://github.com/containerd/containerd/blob/v1.4.4/cmd/ctr/commands/run/run_windows.go
func NewContainer(ctx context.Context, clicontext *cli.Context, client *containerd.Client, dataStore, id string) (containerd.Container, error) {
	var (
		opts  []oci.SpecOpts
		cOpts []containerd.NewContainerOpts
	)

	snapshotter := clicontext.String("snapshotter")
	if snapshotter == "windows-lcow" {
		opts = append(opts, oci.WithDefaultSpecForPlatform("linux/amd64"))
		// Clear the rootfs section.
		opts = append(opts, oci.WithRootFSPath(""))
	} else {
		opts = append(opts, oci.WithDefaultSpec())
		opts = append(opts, oci.WithWindowNetworksAllowUnqualifiedDNSQuery())
		opts = append(opts, oci.WithWindowsIgnoreFlushesDuringBoot())
	}
	if ef := clicontext.String("env-file"); ef != "" {
		opts = append(opts, oci.WithEnvFile(ef))
	}
	opts = append(opts, oci.WithEnv(clicontext.StringSlice("env")))
	var imageVolumes map[string]struct{}
	mountOpts, _, err := generateMountOpts(clicontext, imageVolumes)
	if err != nil {
		return nil, err
	} else {
		opts = append(opts, mountOpts...)
	}

	image, err := imgutil.EnsureImage(ctx, client, clicontext.App.Writer, clicontext.String("snapshotter"), clicontext.Args().First(),
		clicontext.String("pull"), clicontext.Bool("insecure-registry"))
	if err != nil {
		return nil, err
	}

	opts = append(opts, oci.WithImageConfig(image.Image))
	cOpts = append(cOpts, containerd.WithImage(image.Image))
	cOpts = append(cOpts, containerd.WithSnapshotter(image.Snapshotter))
	cOpts = append(cOpts, containerd.WithNewSnapshot(id, image.Image))

	//if len(args) > 0 {
	//	opts = append(opts, oci.WithProcessArgs(args...))
	//}
	if cwd := clicontext.String("cwd"); cwd != "" {
		opts = append(opts, oci.WithProcessCwd(cwd))
	}
	if clicontext.Bool("tty") {
		opts = append(opts, oci.WithTTY)

		con := console.Current()
		size, err := con.Size()
		if err != nil {
			logrus.WithError(err).Error("console size")
		}
		opts = append(opts, oci.WithTTYSize(int(size.Width), int(size.Height)))
	}
	if clicontext.Bool("net-host") {
		return nil, errors.New("Cannot use host mode networking with Windows containers")
	}
	if clicontext.Bool("isolated") {
		opts = append(opts, oci.WithWindowsHyperV)
	}
	limit := clicontext.Uint64("memory-limit")
	if limit != 0 {
		opts = append(opts, oci.WithMemoryLimit(limit))
	}
	ccount := clicontext.Uint64("cpu-count")
	if ccount != 0 {
		opts = append(opts, oci.WithWindowsCPUCount(ccount))
	}

	//cOpts = append(cOpts, containerd.WithContainerLabels(commands.LabelArgs(clicontext.StringSlice("label"))))
	runtime := clicontext.String("runtime")
	var runtimeOpts interface{}
	if runtime == "io.containerd.runhcs.v1" {

		// To avoid nil panic during clicontext.String(),
		// it seems we have to use globalcontext.String()
		lineage := clicontext.Lineage()
		if len(lineage) < 2 {
			return nil, errors.New("error getting global flags")
		}
		globalContext := lineage[len(lineage)-2]
		runtimeOpts = &winopts.Options{
			Debug: globalContext.Bool("debug"),
		}
	}
	cOpts = append(cOpts, containerd.WithRuntime(runtime, runtimeOpts))

	var s specs.Spec
	spec := containerd.WithSpec(&s, opts...)

	cOpts = append(cOpts, spec)

	return client.NewContainer(ctx, id, cOpts...)
}

func runComplete(clicontext *cli.Context) {
	// noop
}

func generateLogUri(flagD bool, dataStore string) (string, error) {
	return "", nil
}
