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
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/dockerconfigresolver"
	"github.com/AkihiroSuda/nerdctl/pkg/imgutil/push"
	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/pkg/errors"

	"github.com/urfave/cli/v2"
)

var pushCommand = &cli.Command{
	Name:      "push",
	Usage:     "Push an image or a repository to a registry",
	ArgsUsage: "NAME[:TAG]",
	Action:    pushAction,
	Flags:     []cli.Flag{},
}

func pushAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 1 {
		return errors.New("image name needs to be specified")
	}
	rawRef := clicontext.Args().First()
	named, err := refdocker.ParseDockerRef(rawRef)
	if err != nil {
		return err
	}
	ref := named.String()
	resolver, err := dockerconfigresolver.New(refdocker.Domain(named))
	if err != nil {
		return err
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	return push.Push(ctx, client, resolver, clicontext.App.Writer, ref, ref)
}
