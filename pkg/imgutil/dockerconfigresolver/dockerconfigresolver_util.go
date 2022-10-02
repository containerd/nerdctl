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

/*
   Portions from https://github.com/moby/moby/blob/v20.10.18/registry/auth.go#L154-L167
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.18/NOTICE
*/

package dockerconfigresolver

import (
	"strings"
)

// IndexServer is used for user auth and image search
//
// From https://github.com/moby/moby/blob/v20.10.18/registry/config.go#L36-L39
const IndexServer = "https://index.docker.io/v1/"

// ConvertToHostname converts a registry url which has http|https prepended
// to just an hostname.
//
// From https://github.com/moby/moby/blob/v20.10.18/registry/auth.go#L154-L167
func ConvertToHostname(url string) string {
	stripped := url
	if strings.HasPrefix(url, "http://") {
		stripped = strings.TrimPrefix(url, "http://")
	} else if strings.HasPrefix(url, "https://") {
		stripped = strings.TrimPrefix(url, "https://")
	}

	nameParts := strings.SplitN(stripped, "/", 2)

	return nameParts[0]
}
