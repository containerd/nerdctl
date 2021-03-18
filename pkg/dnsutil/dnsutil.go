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

package dnsutil

import (
	"bytes"
	"io/ioutil"
	"net"

	"github.com/pkg/errors"
)

func WriteResolvConfFile(path string, dns []string) error {
	var b bytes.Buffer
	if _, err := b.Write([]byte("search localdomain\n")); err != nil {
		return err
	}
	for _, entry := range dns {
		if net.ParseIP(entry) == nil {
			return errors.Errorf("invalid dns %q", entry)
		}
		if _, err := b.Write([]byte("nameserver " + entry + "\n")); err != nil {
			return err
		}
	}
	return ioutil.WriteFile(path, b.Bytes(), 0644)
}
