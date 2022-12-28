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

package hostsstore

import (
	"bytes"
	"strings"
	"testing"

	"gotest.tools/v3/assert"
)

func TestParseHostsButSkipMarkedRegion(t *testing.T) {
	type testCase struct {
		hostsFileContent string
		want             string
	}
	testCases := []testCase{
		{
			hostsFileContent: `
10.4.1.6        outOfMarkedRegion
# <nerdctl>
127.0.0.1       localhost localhost.localdomain
::1             localhost localhost.localdomain
10.4.1.5        35af3f0922a9 35af3f0922a9.etcd-0 alpine-35af3 alpine-35af3.etcd-0
10.4.1.3        993208adcae8 993208adcae8.etcd-0 alpine-99320 alpine-99320.etcd-0
# </nerdctl>
`,
			want: `10.4.1.6        outOfMarkedRegion
`,
		},
		{
			hostsFileContent: `
		# <nerdctl>
		127.0.0.1       localhost localhost.localdomain
		::1             localhost localhost.localdomain
		10.4.1.5        35af3f0922a9 35af3f0922a9.etcd-0 alpine-35af3 alpine-35af3.etcd-0
		10.4.1.3        993208adcae8 993208adcae8.etcd-0 alpine-99320 alpine-99320.etcd-0
		# </nerdctl>
		`,
			want: "",
		},
	}
	for _, tc := range testCases {
		var buf bytes.Buffer
		r := strings.NewReader(tc.hostsFileContent)
		err := parseHostsButSkipMarkedRegion(&buf, r)
		assert.NilError(t, err)
		assert.DeepEqual(t, buf.String(), tc.want)

	}
}
