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
	"github.com/AkihiroSuda/nerdctl/pkg/ocihook"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var internalOCIHookCommand = &cli.Command{
	Name:   "oci-hook",
	Usage:  "OCI hook",
	Action: internalOCIHookAction,
}

func internalOCIHookAction(clicontext *cli.Context) error {
	event := clicontext.Args().First()
	if event == "" {
		return errors.New("event type needs to be passed")
	}
	dataStore, err := getDataStore(clicontext)
	if err != nil {
		return err
	}
	return ocihook.Run(clicontext.App.Reader, clicontext.App.ErrWriter, event,
		dataStore,
		clicontext.String("cni-path"),
		clicontext.String("cni-netconfpath"),
	)
}
