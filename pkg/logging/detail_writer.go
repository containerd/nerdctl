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

package logging

import "io"

type DetailWriter struct {
	w      io.Writer
	prefix string
}

func NewDetailWriter(w io.Writer, prefix string) io.Writer {
	return &DetailWriter{
		w:      w,
		prefix: prefix,
	}
}

func (dw *DetailWriter) Write(p []byte) (n int, err error) {
	if len(p) > 0 {
		if _, err = dw.w.Write([]byte(dw.prefix)); err != nil {
			return 0, err
		}

		return dw.w.Write(p)
	}
	return 0, nil
}
