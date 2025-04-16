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

// Package pty provides a simple to manipulate pty Open method.
// Note that creack is MIT licensed, making it better to depend on it rather than using derived code
// here. Underlying implementation is OK though they have more (unnecessary to us) features and do
// not follow the same coding standards.
package pty

import (
	"errors"
	"os"

	creack "github.com/creack/pty"
)

var (
	// ErrFailure is wrapping system pty creation failure returned by Open().
	ErrFailure = errors.New("pty failure")
	// ErrUnsupportedPlatform is returned by Open() on unsupported platforms.
	ErrUnsupportedPlatform = errors.New("pty not supported on this platform")
)

// Open will allocate and return a new pty.
func Open() (pty, tty *os.File, err error) {
	pty, tty, err = creack.Open()
	if err != nil {
		if errors.Is(err, creack.ErrUnsupported) {
			err = errors.Join(ErrUnsupportedPlatform, err)
		} else {
			err = errors.Join(ErrFailure, err)
		}
	}

	return pty, tty, err
}
