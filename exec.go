/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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
	"io"
	"os"

	"github.com/AkihiroSuda/nerdctl/pkg/idutil/containerwalker"
	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/cmd/ctr/commands"
	"github.com/containerd/containerd/cmd/ctr/commands/tasks"
	"github.com/containerd/containerd/oci"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
)

var execCommand = &cli.Command{
	Name:      "exec",
	Usage:     "Run a command in a running container",
	ArgsUsage: "[flags] CONTAINER",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "tty",
			Aliases: []string{"t"},
			Usage:   "(Currently -t needs to correspond to -i)",
		},
		&cli.BoolFlag{
			Name:    "interactive",
			Aliases: []string{"i"},
			Usage:   "Keep STDIN open even if not attached",
		},
		&cli.BoolFlag{
			Name:    "detach",
			Aliases: []string{"d"},
			Usage:   "Detached mode: run command in the background",
		},
		&cli.StringFlag{
			Name:    "workdir",
			Aliases: []string{"w"},
			Usage:   "Working directory inside the container",
		},
		&cli.StringSliceFlag{
			Name:    "env",
			Aliases: []string{"e"},
			Usage:   "Set environment variables",
		},
		&cli.BoolFlag{
			Name:  "privileged",
			Usage: "Give extended privileges to the command",
		},
	},
	Action: execAction,
}

func execAction(clicontext *cli.Context) error {
	if clicontext.NArg() < 2 {
		return errors.Errorf("requires at least 2 arguments")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchIndex > 1 {
				return errors.Errorf("ambiguous ID %q", found.Req)
			}
			return execActionWithContainer(ctx, clicontext, found.Container)
		},
	}
	req := clicontext.Args().First()
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return errors.Errorf("no such container %s", req)
	}
	return nil
}

func execActionWithContainer(ctx context.Context, clicontext *cli.Context, container containerd.Container) error {
	flagI := clicontext.Bool("i")
	flagT := clicontext.Bool("t")
	flagD := clicontext.Bool("d")

	if flagI {
		if flagD {
			return errors.New("currently flag -i and -d cannot be specified together (FIXME)")
		}
	}

	if flagT {
		if flagD {
			return errors.New("currently flag -t and -d cannot be specified together (FIXME)")
		}
		if !flagI {
			return errors.New("currently flag -t needs -i to be specified together (FIXME)")
		}
	}

	pspec, err := generateExecProcessSpec(ctx, clicontext, container)
	if err != nil {
		return err
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return err
	}
	var (
		ioCreator cio.Creator
		in        io.Reader
		stdinC    = &stdinCloser{
			stdin: os.Stdin,
		}
	)

	if flagI {
		in = stdinC
	}
	cioOpts := []cio.Opt{cio.WithStreams(in, os.Stdout, os.Stderr)}
	if flagT {
		cioOpts = append(cioOpts, cio.WithTerminal)
	}
	ioCreator = cio.NewCreator(cioOpts...)

	execID := "exec-" + genID()
	process, err := task.Exec(ctx, execID, pspec, ioCreator)
	if err != nil {
		return err
	}
	stdinC.closer = func() {
		process.CloseIO(ctx, containerd.WithStdinCloser)
	}
	// if detach, we should not call this defer
	if !flagD {
		defer process.Delete(ctx)
	}

	statusC, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	var con console.Console
	if flagT {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}
	if !flagD {
		if flagT {
			if err := tasks.HandleConsoleResize(ctx, process, con); err != nil {
				logrus.WithError(err).Error("console resize")
			}
		} else {
			sigc := commands.ForwardAllSignals(ctx, process)
			defer commands.StopCatch(sigc)
		}
	}

	if err := process.Start(ctx); err != nil {
		return err
	}
	if flagD {
		return nil
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.NewExitError("", int(code))
	}
	return nil
}

func generateExecProcessSpec(ctx context.Context, clicontext *cli.Context, container containerd.Container) (*specs.Process, error) {
	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	}

	pspec := spec.Process
	pspec.Terminal = clicontext.Bool("t")
	pspec.Args = clicontext.Args().Tail()

	if workdir := clicontext.String("workdir"); workdir != "" {
		pspec.Cwd = workdir
	}
	for _, e := range clicontext.StringSlice("env") {
		pspec.Env = append(pspec.Env, e)
	}

	if clicontext.Bool("privileged") {
		if pspec.Capabilities == nil {
			pspec.Capabilities = &specs.LinuxCapabilities{}
		}
		pspec.Capabilities.Bounding = oci.GetAllCapabilities()
		pspec.Capabilities.Permitted = pspec.Capabilities.Bounding
		pspec.Capabilities.Inheritable = pspec.Capabilities.Bounding
		pspec.Capabilities.Effective = pspec.Capabilities.Bounding

		// https://github.com/moby/moby/pull/36466/files
		// > `docker exec --privileged` does not currently disable AppArmor
		// > profiles. Privileged configuration of the container is inherited
	}

	return pspec, nil
}
