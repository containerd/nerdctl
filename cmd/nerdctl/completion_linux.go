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
	"github.com/containerd/nerdctl/v2/pkg/apparmorutil"
	"github.com/spf13/cobra"
)

func shellCompleteApparmorProfiles(cmd *cobra.Command) ([]string, cobra.ShellCompDirective) {
	profiles, err := apparmorutil.Profiles()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string // nolint: prealloc
	for _, f := range profiles {
		names = append(names, f.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
