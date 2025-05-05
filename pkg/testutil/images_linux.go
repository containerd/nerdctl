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

package testutil

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

//go:embed images.env
var imagesList string

// getImage retrieves (from the env file) the fully qualified reference of an image, from its short name.
// No particular error handling effort is done here.
// If the env file is broken, or if the requested image does not exist, this is fatal.
func getImage(name string) string {
	parsed, err := referenceutil.Parse(name)

	if err != nil {
		panic(fmt.Errorf("malformed image name requested: %w", err))
	}

	// Ignore tag and digests for now - we currently support only one tag digest per image
	name = parsed.Domain + "/" + parsed.Path

	for _, k := range strings.Split(imagesList, "\n") {
		k = strings.TrimSpace(k)
		if strings.HasPrefix(k, "#") || k == "" {
			continue
		}

		spl := strings.Split(k, "=")
		if len(spl) != 2 {
			continue
		}

		item, err := referenceutil.Parse(spl[1])
		if err != nil {
			panic(fmt.Errorf("malformed image found in env file: %w (%s)", err, spl[1]))
		}

		if item.Domain+"/"+item.Path == name {
			return spl[1]
		}
	}

	panic(fmt.Sprintf("no such test image is defined: %s", name))

	//nolint:govet
	return ""
}
