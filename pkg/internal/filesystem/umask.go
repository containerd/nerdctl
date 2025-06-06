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

package filesystem

import (
	"math"
	"sync"
)

var (
	mu    sync.Mutex
	cMask = -1
)

// GetUmask retrieves the current umask.
func GetUmask() uint32 {
	if cMask != -1 {
		return uint32(cMask)
	}

	mu.Lock()
	defer mu.Unlock()

	cMask = umask(0)

	// FIXME: one day... we will get rid of 32 bits arm...
	cMask64 := int64(cMask)
	if cMask64 > math.MaxUint32 || cMask < 0 {
		panic("currently set user umask is out of range")
	}

	_ = umask(cMask)

	return uint32(cMask)
}
