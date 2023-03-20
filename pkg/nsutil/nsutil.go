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

// Package nsutil provides utilities for namespaces.
package nsutil

import (
	"fmt"
	"strings"
)

// Ensures the provided namespace name is valid.
// Namespace names cannot be path-like strings or pre-defined aliases such as "..".
// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#path-segment-names
func ValidateNamespaceName(nsName string) error {
	if nsName == "" {
		return fmt.Errorf("namespace name cannot be empty")
	}

	// Slash and '$' for POSIX and backslash and '%' for Windows.
	pathSeparators := "/\\%$"
	if strings.ContainsAny(nsName, pathSeparators) {
		return fmt.Errorf("namespace name cannot contain any special characters (%q): %s", pathSeparators, nsName)
	}

	specialAliases := []string{".", "..", "~"}
	for _, alias := range specialAliases {
		if nsName == alias {
			return fmt.Errorf("namespace name cannot be special path alias %q", alias)
		}
	}

	return nil
}
