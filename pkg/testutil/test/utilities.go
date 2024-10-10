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

package test

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// RandomStringBase64 generates a base64 encoded random string
func RandomStringBase64(n int) string {
	b := make([]byte, n)
	l, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	if l != n {
		panic(fmt.Errorf("expected %d bytes, got %d bytes", n, l))
	}
	return base64.URLEncoding.EncodeToString(b)
}
