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
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/plugin"
	runcoptions "github.com/containerd/containerd/runtime/v2/runc/options"
	"github.com/urfave/cli/v2"
)

func generateRuntimeCOpts(clicontext *cli.Context) ([]containerd.NewContainerOpts, error) {
	runtime := plugin.RuntimeRuncV2
	var (
		runcOpts    runcoptions.Options
		runtimeOpts interface{} = &runcOpts
	)
	if runtimeStr := clicontext.String("runtime"); runtimeStr != "" {
		if strings.HasPrefix(runtimeStr, "io.containerd.") {
			runtime = runtimeStr
			if !strings.HasPrefix(runtimeStr, "io.containerd.runc.") {
				runtimeOpts = nil
			}
		} else {
			// runtimeStr is a runc binary
			runcOpts.BinaryName = runtimeStr
		}
	}
	o := containerd.WithRuntime(runtime, runtimeOpts)
	return []containerd.NewContainerOpts{o}, nil
}
