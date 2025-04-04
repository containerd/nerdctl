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

package formatter

import "fmt"

// OSC8 hyperlinks implementation.
type OSC8 struct {
	Location string `json:"location"`
	Line     int    `json:"line"`
	Text     string `json:"text"`
}

func (o *OSC8) String() string {
	// FIXME: not sure if any desktop software does support line numbers anchors?
	// FIXME: test that the terminal is able to display these and fallback to printing the information if not.
	return fmt.Sprintf("\x1b]8;;%s#%d:1\x07%s\x1b]8;;\x07"+"\u001b[0m", o.Location, o.Line, o.Text)
}
