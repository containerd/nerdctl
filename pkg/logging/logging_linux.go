//go:build linux
// +build linux

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
	"os"

	"golang.org/x/sys/unix"
)

func waitIOClose(reader io.Reader) (chan struct{}, error) {
	closeIO := make(chan struct{})
	epfd, err := unix.EpollCreate1(0)
	if err != nil {
		return nil, err
	}
	file, ok := reader.(*os.File)
	if !ok {
		return nil, fmt.Errorf("reader is not an cancelreader.File")
	}
	fd := file.Fd()
	event := unix.EpollEvent{
		Events: unix.EPOLLHUP,
		Fd:     int32(fd),
	}
	if err := unix.EpollCtl(epfd, unix.EPOLL_CTL_ADD, int(fd), &event); err != nil {
		return nil, err
	}
	events := make([]unix.EpollEvent, 1)
	go func() {
		for {
			n, err := unix.EpollWait(epfd, events, -1)
			if err != nil {
				continue
			}
			for i := 0; i < n; i++ {
				if events[i].Events&unix.EPOLLHUP != 0 {
					close(closeIO)
					return
				}
			}
		}
	}()
	return closeIO, nil
}
