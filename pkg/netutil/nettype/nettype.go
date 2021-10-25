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

package nettype

import "fmt"

type Type int

const (
	Invalid Type = iota
	None
	Host
	CNI
)

var netTypeToName = map[interface{}]string{
	Invalid: "invalid",
	None:    "none",
	Host:    "host",
	CNI:     "cni",
}

func Detect(names []string) (Type, error) {
	var res Type

	for _, name := range names {
		var tmp Type
		switch name {
		case "none":
			tmp = None
		case "host":
			tmp = Host
		default:
			tmp = CNI
		}
		if res != Invalid && res != tmp {
			return Invalid, fmt.Errorf("mixed network types: %v and %v", netTypeToName[res], netTypeToName[tmp])
		}
		res = tmp
	}

	// defaults to CNI
	if res == Invalid {
		res = CNI
	}

	return res, nil
}
