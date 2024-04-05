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

// Package annotations defines OCI annotations
package annotations

const (
	// Prefix is the common prefix of nerdctl annotations
	Prefix = "nerdctl/"

	// Bypass4netns is the flag for acceleration with bypass4netns
	// Boolean value which can be parsed with strconv.ParseBool() is required.
	// (like "nerdctl/bypass4netns=true" or "nerdctl/bypass4netns=false")
	Bypass4netns = Prefix + "bypass4netns"

	// Bypass4netnsIgnoreSubnets is a JSON of []string that is appended to
	// the `bypass4netns --ignore` list.
	Bypass4netnsIgnoreSubnets = Bypass4netns + "-ignore-subnets"

	// Bypass4netnsIgnoreBind disables acceleration for bind.
	// Boolean value which can be parsed with strconv.ParseBool() is required.
	Bypass4netnsIgnoreBind = Bypass4netns + "-ignore-bind"
)

var ShellCompletions = []string{
	Bypass4netns + "=true",
	Bypass4netns + "=false",
	Bypass4netnsIgnoreSubnets + "=",
	Bypass4netnsIgnoreBind + "=true",
	Bypass4netnsIgnoreBind + "=false",
}
