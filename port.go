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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/AkihiroSuda/nerdctl/pkg/idutil/containerwalker"
	"github.com/AkihiroSuda/nerdctl/pkg/labels"
	"github.com/containerd/containerd"
	gocni "github.com/containerd/go-cni"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var portCommand = &cli.Command{
	Name:      "port",
	Usage:     "List port mappings or a specific mapping for the container",
	ArgsUsage: "CONTAINER [PRIVATE_PORT[/PROTO]]",
	Action:    portAction,
}

func portAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 && clicontext.NArg() != 2 {
		return errors.Errorf("requires at least 1 and at most 2 arguments")
	}

	argPort := -1
	argProto := ""

	if portProto := clicontext.Args().Get(1); portProto != "" {
		splitBySlash := strings.Split(portProto, "/")
		var err error
		argPort, err = strconv.Atoi(splitBySlash[0])
		if err != nil {
			return err
		}
		if argPort <= 0 {
			return errors.Errorf("unexpected port %d", argPort)
		}
		switch len(splitBySlash) {
		case 1:
			argProto = "tcp"
		case 2:
			argProto = strings.ToLower(splitBySlash[1])
		default:
			return errors.Errorf("failed to parse %q", portProto)
		}
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return errors.Errorf("ambiguous ID %q", found.Req)
			}
			return printPort(ctx, clicontext, found.Container, argPort, argProto)
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

func printPort(ctx context.Context, clicontext *cli.Context, container containerd.Container, argPort int, argProto string) error {
	l, err := container.Labels(ctx)
	if err != nil {
		return err
	}
	portsJSON := l[labels.Ports]
	if portsJSON == "" {
		return nil
	}
	var ports []gocni.PortMapping
	if err := json.Unmarshal([]byte(portsJSON), &ports); err != nil {
		return err
	}

	if argPort < 0 {
		for _, p := range ports {
			fmt.Fprintf(clicontext.App.Writer, "%d/%s -> %s:%d\n", p.ContainerPort, p.Protocol, p.HostIP, p.HostPort)
		}
		return nil
	}

	for _, p := range ports {
		if p.ContainerPort == int32(argPort) && strings.ToLower(p.Protocol) == argProto {
			fmt.Fprintf(clicontext.App.Writer, "%s:%d\n", p.HostIP, p.HostPort)
			return nil
		}
	}
	return errors.Errorf("no public port %d/%s published for %q", argPort, argProto, container.ID())
}
