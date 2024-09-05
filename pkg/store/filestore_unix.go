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

package store

import (
	"fmt"
	"regexp"
)

// Note that Darwin has different restrictions - though, we do not support Darwin at this point...
// https://stackoverflow.com/questions/1976007/what-characters-are-forbidden-in-windows-and-linux-directory-names
var (
	disallowedKeywords = regexp.MustCompile(`^([.]|[.][.])$`)
	reservedCharacters = regexp.MustCompile(`[\x{0}/]`)
)

func validatePlatformSpecific(pathComponent string) error {
	if reservedCharacters.MatchString(pathComponent) {
		return fmt.Errorf("identifier %q cannot contain any of the following characters: %q", pathComponent, reservedCharacters)
	}

	if disallowedKeywords.MatchString(pathComponent) {
		return fmt.Errorf("identifier %q cannot be any of the reserved keywords: %q", pathComponent, disallowedKeywords)
	}

	return nil
}
