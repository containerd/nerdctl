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

package mimicry

import (
	"strings"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/formatter"
)

const (
	maxLineLength       = 110
	sourceLineAround    = 2
	breakpointDecorator = "🔴"
	frameDecorator      = "⬆️"
)

// PrintCall does fancy format a Call.
func PrintCall(call *Call) string {
	sectionSeparator := strings.Repeat("_", maxLineLength)

	debug := [][]any{
		{"Arguments", call.Args},
		{"Time", call.Time.Format(time.RFC3339)},
	}

	output := []string{
		formatter.Table(debug, "-"),
		sectionSeparator,
	}

	marker := breakpointDecorator
	for _, v := range call.Frames {
		output = append(output,
			v.String(),
			sectionSeparator,
			v.Excerpt(sourceLineAround, marker),
			sectionSeparator,
		)
		marker = frameDecorator
	}

	return "\n" + strings.Join(output, "\n")
}
