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

package utils

import (
	"crypto/rand"
	"encoding/base64"
)

// RandomStringBase64 generates a base64 encoded random string.
func RandomStringBase64(desiredLength int) string {
	randomBytes := make([]byte, desiredLength)

	randomLength, err := rand.Read(randomBytes)
	if err != nil {
		panic(err)
	}

	if randomLength != desiredLength {
		panic("rand failing")
	}

	return base64.URLEncoding.EncodeToString(randomBytes)
}
