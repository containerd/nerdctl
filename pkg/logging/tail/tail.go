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

/*
	Forked from https://github.com/kubernetes/kubernetes/blob/master/pkg/util/tail/tail.go
	Copyright The Kubernetes Authors.
	Licensed under the Apache License, Version 2.0
*/

package tail

import (
	"bytes"
	"io"
)

const (
	// blockSize is the block size used in tail.
	blockSize = 1024
)

var (
	// eol is the end-of-line sign in the log.
	eol = []byte{'\n'}
)

// FindTailLineStartIndex returns the start of last nth line.
// * If n <= 0, return the beginning of the file.
// * If n > 0, return the beginning of last nth line.
// Notice that if the last line is incomplete (no end-of-line), it will not be counted
// as one line.
func FindTailLineStartIndex(f io.ReadSeeker, n uint) (int64, error) {
	if n <= 0 {
		return 0, nil
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	var left, cnt int64
	buf := make([]byte, blockSize)
	for right := size; right > 0 && uint(cnt) <= n; right -= blockSize {
		left = right - blockSize
		if left < 0 {
			left = 0
			buf = make([]byte, right)
		}
		if _, err := f.Seek(left, io.SeekStart); err != nil {
			return 0, err
		}
		if _, err := f.Read(buf); err != nil {
			return 0, err
		}
		cnt += int64(bytes.Count(buf, eol))
	}
	for ; uint(cnt) > n; cnt-- {
		idx := bytes.Index(buf, eol) + 1
		buf = buf[idx:]
		left += int64(idx)
	}
	return left, nil
}
