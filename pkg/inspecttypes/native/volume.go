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

package native

// Volume is also compatible with Docker
type Volume struct {
	Name       string             `json:"Name"`
	Mountpoint string             `json:"Mountpoint"`
	Labels     *map[string]string `json:"Labels,omitempty"`
	Size       int64              `json:"Size,omitempty"`
}
