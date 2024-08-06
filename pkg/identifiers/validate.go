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

// Package identifiers implements functions for docker compatible identifier validation.
package identifiers

import (
	"fmt"
	"regexp"

	"github.com/containerd/errdefs"
)

const AllowedIdentfierChars = `[a-zA-Z0-9][a-zA-Z0-9_.-]`

var AllowedIdentifierPattern = regexp.MustCompile(`^` + AllowedIdentfierChars + `+$`)

// ValidateDockerCompat implements docker compatible identifier validation.
// The containerd implementation allows single character identifiers, while the
// Docker compatible implementation requires at least 2 characters for identifiers.
// The containerd implementation enforces a maximum length constraint of 76 characters,
// while the Docker compatible implementation omits the length check entirely.
func ValidateDockerCompat(s string) error {
	if len(s) == 0 {
		return fmt.Errorf("identifier must not be empty %w", errdefs.ErrInvalidArgument)
	}

	if !AllowedIdentifierPattern.MatchString(s) {
		return fmt.Errorf("identifier %q must match pattern %q: %w", s, AllowedIdentfierChars, errdefs.ErrInvalidArgument)
	}
	return nil
}
