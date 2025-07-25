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

package completion

import "github.com/spf13/cobra"

func NamespaceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func SnapshotterNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func CgroupManagerNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func NetworkDrivers(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	candidates := []string{"nat"}
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func IPAMDrivers(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"default"}, cobra.ShellCompDirectiveNoFileComp
}

func NetworkOptions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	driver, _ := cmd.Flags().GetString("driver")
	if driver == "" {
		driver = "nat"
	}

	var candidates []string
	switch driver {
	case "nat":
		candidates = []string{
			"mtu=",
			"com.docker.network.driver.mtu=",
		}
	default:
		candidates = []string{
			"mtu=",
			"com.docker.network.driver.mtu=",
		}
	}
	return candidates, cobra.ShellCompDirectiveNoSpace
}
