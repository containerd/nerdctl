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

package types

import "io"

// NamespaceCreateOptions specifies options for `nerdctl namespace create`.
type NamespaceCreateOptions struct {
	GOptions GlobalCommandOptions
	// Labels are the namespace labels
	Labels []string
}

// NamespaceUpdateOptions specifies options for `nerdctl namespace update`.
type NamespaceUpdateOptions NamespaceCreateOptions

// NamespaceRemoveOptions specifies options for `nerdctl namespace rm`.
type NamespaceRemoveOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// CGroup delete the namespace's cgroup
	CGroup bool
}

// NamespaceInspectOptions specifies options for `nerdctl namespace inspect`.
type NamespaceInspectOptions struct {
	Stdout   io.Writer
	GOptions GlobalCommandOptions
	// Format the output using the given Go template, e.g, '{{json .}}'
	Format string
}
