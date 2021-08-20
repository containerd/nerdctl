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

package main

// Flusher is implemented by text/tabwriter.Writer
type Flusher interface {
	Flush() error
}

func formatLabels(labels map[string]string) string {
	var res string
	for k, v := range labels {
		s := k + "=" + v
		if res == "" {
			res = s
		} else {
			res += "," + s
		}
	}
	return res
}
