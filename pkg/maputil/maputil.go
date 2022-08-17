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

package maputil

import (
	"fmt"
	"strconv"
)

// MapBoolValueAsOpt will parse key as a command-line option.
// If only key is specified will be treated as true,
// otherwise, the value will be parsed and returned.
// This is useful when command line flags have options.
// Following examples illustrate this:
// --security-opt xxx returns true
// --security-opt xxx=true returns true
// --security-opt xxx=false returns false
// --security-opt xxx=invalid returns false and error
func MapBoolValueAsOpt(m map[string]string, key string) (bool, error) {
	if str, ok := m[key]; ok {
		if str == "" {
			return true, nil
		} else {
			b, err := strconv.ParseBool(str)
			if err != nil {
				return false, fmt.Errorf("invalid \"%s\" value: %q: %w", key, str, err)
			}
			return b, nil
		}
	}

	return false, nil
}
