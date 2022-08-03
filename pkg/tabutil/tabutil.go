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

package tabutil

import (
	"fmt"
	"strings"
)

type TabReader struct {
	indexs  map[string]*headerIndex
	headers []string
}

type headerIndex struct {
	start int
	end   int
}

func NewReader(header string) *TabReader {
	headers := strings.Split(strings.TrimSpace(header), "\t")
	return &TabReader{
		indexs:  make(map[string]*headerIndex),
		headers: headers,
	}
}

func (r *TabReader) ParseHeader(header string) error {
	if len(r.headers) == 0 {
		return fmt.Errorf("no header")
	}
	for i := range r.headers {
		start := strings.Index(header, r.headers[i])
		if start == -1 {
			return fmt.Errorf("header %q not matched", r.headers[i])
		}
		if i > 0 {
			r.indexs[r.headers[i-1]].end = start
		}
		r.indexs[r.headers[i]] = &headerIndex{start, -1}
	}
	return nil
}

func (r *TabReader) ReadRow(row, key string) (string, bool) {
	idx, ok := r.indexs[key]
	if !ok {
		return "", false
	}
	if idx.start > len(row) {
		return "", false
	}
	var value string
	if idx.end == -1 {
		value = row[idx.start:]
	} else {
		end := min(idx.end, len(row))
		value = row[idx.start:end]
	}
	return strings.TrimSpace(value), true
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
