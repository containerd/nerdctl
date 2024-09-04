//go:build unix

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
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var errFileSpecDoesntMatchFormat = errors.New("filespec must match the canonical format: [container:]file/path")

func parseCpFileSpec(arg string) (*cpFileSpec, error) {
	i := strings.Index(arg, ":")

	// filespec starting with a semicolon is invalid
	if i == 0 {
		return nil, errFileSpecDoesntMatchFormat
	}

	if filepath.IsAbs(arg) {
		// Explicit local absolute path, e.g., `C:\foo` or `/foo`.
		return &cpFileSpec{
			Container: nil,
			Path:      arg,
		}, nil
	}

	parts := strings.SplitN(arg, ":", 2)

	if len(parts) == 1 || strings.HasPrefix(parts[0], ".") {
		// Either there's no `:` in the arg
		// OR it's an explicit local relative path like `./file:name.txt`.
		return &cpFileSpec{
			Path: arg,
		}, nil
	}

	return &cpFileSpec{
		Container: &parts[0],
		Path:      parts[1],
	}, nil
}

type cpFileSpec struct {
	Container *string
	Path      string
}

func cpShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterFileExt
}
