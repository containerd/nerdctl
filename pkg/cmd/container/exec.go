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

package container

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/containerd/console"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/containerd/nerdctl/v2/pkg/flagutil"
	"github.com/containerd/nerdctl/v2/pkg/idgen"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/signalutil"
	"github.com/containerd/nerdctl/v2/pkg/taskutil"
)

// Exec will find the right running container to run a new command.
func Exec(ctx context.Context, client *containerd.Client, args []string, options types.ContainerExecOptions) error {
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			return execActionWithContainer(ctx, client, found.Container, args, options)
		},
	}
	req := args[0]
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", req)
	}
	return nil
}

func execActionWithContainer(ctx context.Context, client *containerd.Client, container containerd.Container, args []string, options types.ContainerExecOptions) error {
	pspec, err := generateExecProcessSpec(ctx, client, container, args, options)
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
		stdinC    = &taskutil.StdinCloser{
			Stdin: os.Stdin,
		}
	)

	if options.Interactive {
		in = stdinC
	}
	cioOpts := []cio.Opt{cio.WithStreams(in, os.Stdout, os.Stderr)}
	if options.TTY {
		cioOpts = append(cioOpts, cio.WithTerminal)
	}
	ioCreator = cio.NewCreator(cioOpts...)

	execID := "exec-" + idgen.GenerateID()
	process, err := task.Exec(ctx, execID, pspec, ioCreator)
	if err != nil {
		return err
	}
	stdinC.Closer = func() {
		process.CloseIO(ctx, containerd.WithStdinCloser)
	}
	// if detach, we should not call this defer
	if !options.Detach {
		defer process.Delete(ctx)
	}

	statusC, err := process.Wait(ctx)
	if err != nil {
		return err
	}

	var con console.Console
	if options.TTY {
		con = console.Current()
		defer con.Reset()
		if err := con.SetRaw(); err != nil {
			return err
		}
	}
	if !options.Detach {
		if options.TTY {
			if err := consoleutil.HandleConsoleResize(ctx, process, con); err != nil {
				log.G(ctx).WithError(err).Error("console resize")
			}
		} else {
			sigc := signalutil.ForwardAllSignals(ctx, process)
			defer signalutil.StopCatch(sigc)
		}
	}

	if err := process.Start(ctx); err != nil {
		return err
	}
	if options.Detach {
		return nil
	}
	status := <-statusC
	code, _, err := status.Result()
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exec failed with exit code %d", code)
	}
	return nil
}

func generateExecProcessSpec(ctx context.Context, client *containerd.Client, container containerd.Container, args []string, options types.ContainerExecOptions) (*specs.Process, error) {
	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	}
	userOpts, err := generateUserOpts(options.User)
	if err != nil {
		return nil, err
	}
	if userOpts != nil {
		c, err := container.Info(ctx)
		if err != nil {
			return nil, err
		}
		for _, opt := range userOpts {
			if err := opt(ctx, client, &c, spec); err != nil {
				return nil, err
			}
		}
	}

	pspec := spec.Process
	pspec.Terminal = options.TTY
	if pspec.Terminal {
		if size, err := console.Current().Size(); err == nil {
			pspec.ConsoleSize = &specs.Box{Height: uint(size.Height), Width: uint(size.Width)}
		}
	}
	pspec.Args = args[1:]

	if options.Workdir != "" {
		pspec.Cwd = options.Workdir
	}
	envs, err := flagutil.MergeEnvFileAndOSEnv(options.EnvFile, options.Env)
	if err != nil {
		return nil, err
	}
	pspec.Env = flagutil.ReplaceOrAppendEnvValues(pspec.Env, envs)

	if options.Privileged {
		err = setExecCapabilities(pspec)
		if err != nil {
			return nil, err
		}
	}

	return pspec, nil
}
