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

package namespace

import (
	"strings"
	"unicode"
    "errors"
)

func objectWithLabelArgs(args []string) map[string]string {
	if len(args) >= 1 {
		return labelArgs(args)
	}
	return nil
}

// labelArgs returns a map of label key,value pairs.
// From https://github.com/containerd/containerd/blob/v1.7.0-rc.2/cmd/ctr/commands/commands.go#L229-L241
func labelArgs(labelStrings []string) map[string]string {
	labels := make(map[string]string, len(labelStrings))
	for _, label := range labelStrings {
		key, value, ok := strings.Cut(label, "=")
		if !ok {
			value = "true"
		}
		labels[key] = value
	}

	return labels
}

// Returns an error if name is invalid.
func validateNamespaceName(name string) error {
    for _, c := range(name) {
        if (
            c == ' ' ||
            unicode.IsLower(c) ||
            unicode.IsNumber(c)) {
            continue
        }

        return errors.New("invalid namespace name - use only lowercase alphanumeric characters and hyphens")
    }

    return nil
}
