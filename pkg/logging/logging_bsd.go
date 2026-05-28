//go:build darwin || freebsd || netbsd || openbsd || dragonfly
// +build darwin freebsd netbsd openbsd dragonfly

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

import (
	"fmt"
	"io"

	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"
)

func waitIOClose(reader io.Reader) (chan struct{}, error) {
	closeIO := make(chan struct{})
	file, ok := reader.(cancelreader.File)
	if !ok {
		return nil, fmt.Errorf("reader is not an cancelreader.File")
	}

	kq, err := unix.Kqueue()
	if err != nil {
		return nil, fmt.Errorf("create kqueue: %w", err)
	}
	kev := unix.Kevent_t{
		Ident:  uint64(file.Fd()),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE,
	}

	events := make([]unix.Kevent_t, 1)
	_, err = unix.Kevent(kq, []unix.Kevent_t{kev}, events, nil)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			n, err := unix.Kevent(kq, nil, events, nil)
			if err != nil {
				continue
			}
			for i := 0; i < n; i++ {
				if events[i].Flags&unix.EV_EOF != 0 {
					close(closeIO)
					return
				}
			}
		}
	}()
	return closeIO, nil
}
