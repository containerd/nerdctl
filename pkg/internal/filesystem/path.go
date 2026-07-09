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

package filesystem

import (
	"errors"
	"strings"
)

var (
	errForbiddenChars     = errors.New("forbidden characters in path component")
	errForbiddenKeywords  = errors.New("forbidden keywords in path component")
	errInvalidPathTooLong = errors.New("path component must be shorter than 256 characters")
	errInvalidPathEmpty   = errors.New("path component cannot be empty")
)

// ValidatePathComponent will enforce os specific filename restrictions on a single path component.
func ValidatePathComponent(pathComponent string) error {
	// https://en.wikipedia.org/wiki/Comparison_of_file_systems#Limits
	if len(pathComponent) > pathComponentMaxLength {
		return errors.Join(ErrInvalidPath, errInvalidPathTooLong)
	}

	if strings.TrimSpace(pathComponent) == "" {
		return errors.Join(ErrInvalidPath, errInvalidPathEmpty)
	}

	if err := validatePlatformSpecific(pathComponent); err != nil {
		return errors.Join(ErrInvalidPath, err)
	}

	return nil
}
