//go:build !(linux || freebsd)

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

package attachmux

import (
	"context"
	"net"
)

// Probe is not implemented on this platform, so callers keep the legacy path.
func Probe(string) error { return ErrUnsupported }

// Listen is not implemented on this platform.
func Listen(string) (net.Listener, error) { return nil, ErrUnsupported }

// Dial is not implemented on this platform.
func Dial(context.Context, string) (*Session, error) { return nil, ErrUnsupported }

// DialStarting is not implemented on this platform.
func DialStarting(context.Context, string) (*Session, error) { return nil, ErrUnsupported }

// RemoveSocket is not implemented on this platform.
func RemoveSocket(string) error { return nil }
