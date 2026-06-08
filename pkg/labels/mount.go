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

package labels

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SetMount parses a JSON array of mounts and stores each individual mount object as an indexed label.
func SetMount(m map[string]string, jsonMountBytes []byte) error {
	if len(jsonMountBytes) == 0 || string(jsonMountBytes) == "null" || string(jsonMountBytes) == "[]" {
		return nil
	}

	var rawMounts []json.RawMessage
	if err := json.Unmarshal(jsonMountBytes, &rawMounts); err != nil {
		// Fallback: If it's somehow not a slice, write the whole thing to the legacy key
		m[Mounts] = string(jsonMountBytes)
		return nil
	}

	for i, rawMount := range rawMounts {
		key := fmt.Sprintf(MountsKeyFormat, i)
		m[key] = string(rawMount)
	}

	return nil
}

// GetMount extracts and reassembles indexed mount metadata back into a single JSON array string
func GetMount(containerLabels map[string]string) string {
	// Try legacy label first for backward compatibility
	if legacyMount, ok := containerLabels[Mounts]; ok && legacyMount != "" {
		return legacyMount
	}

	var rawMounts []string

	for i := 0; i < len(containerLabels); i++ {
		key := fmt.Sprintf(MountsKeyFormat, i)
		chunk, found := containerLabels[key]

		if !found || chunk == "" {
			break
		}
		rawMounts = append(rawMounts, chunk)
	}

	if len(rawMounts) > 0 {
		return "[" + strings.Join(rawMounts, ",") + "]"
	}

	return ""
}
