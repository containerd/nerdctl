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

package identifiers

import (
	"fmt"
	"regexp"

	"github.com/containerd/errdefs"
)

// See https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file

// Note that ¹²³ is technically not required here since they are out of the regexp for regular validation
var windowsDisallowed = regexp.MustCompile(`^(con|prn|nul|aux|com[1-9¹²³]|lpt[1-9¹²³])([.].*)?`)

func validatePlatformSpecific(identifier string) error {
	if windowsDisallowed.MatchString(identifier) {
		return fmt.Errorf("identifier %q must not match reserved windows pattern %q: %w", identifier, windowsDisallowed, errdefs.ErrInvalidArgument)
	}

	if identifier[len(identifier)-1:] == "." {
		return fmt.Errorf("windows does not allow identifiers ending with a dot or space (%q)", identifier)
	}

	return nil
}
