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

package transferutil

import (
	"context"
	"strings"
	"sync"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/streaming"
	streamproxy "github.com/containerd/containerd/v2/core/streaming/proxy"
	"github.com/containerd/containerd/v2/core/transfer"
	"github.com/containerd/containerd/v2/core/transfer/proxy"
	"github.com/containerd/typeurl/v2"
)

// Transfer is like (*containerd.Client).Transfer, except that it does not return before the progress function
// (transfer.WithProgress) has been called with every progress event.
//
// (*containerd.Client).Transfer receives the progress events in a goroutine that it does not wait for,
// so the last events, such as the "saved" events of the images, may otherwise be delivered after it returned.
func Transfer(ctx context.Context, client *containerd.Client, src, dst any, opts ...transfer.Opt) error {
	ctx, done, err := client.WithLease(ctx)
	if err != nil {
		return err
	}
	defer done(ctx)

	sc := &waitingStreamCreator{StreamCreator: streamproxy.NewStreamCreator(client.Conn())}
	err = proxy.NewTransferrer(client.Conn(), sc).Transfer(ctx, src, dst, opts...)

	// The daemon closes the progress stream before replying, so this does not block for long.
	// The timeout is only a safety net, for a daemon that would not.
	waitC := make(chan struct{})
	go func() {
		sc.wg.Wait()
		close(waitC)
	}()
	select {
	case <-waitC:
	case <-ctx.Done():
	case <-time.After(progressStreamTimeout):
	}
	return err
}

const progressStreamTimeout = 10 * time.Second

// waitingStreamCreator tracks the progress streams it creates, until their receiving end gets an error
// (typically io.EOF). The other streams, such as the one sending an archive to import, are not tracked.
type waitingStreamCreator struct {
	streaming.StreamCreator
	wg sync.WaitGroup
}

func (sc *waitingStreamCreator) Create(ctx context.Context, id string) (streaming.Stream, error) {
	stream, err := sc.StreamCreator.Create(ctx, id)
	if err != nil {
		return nil, err
	}
	// See GenerateID("progress") in (*proxyTransferrer).Transfer.
	if !strings.HasPrefix(id, "progress-") {
		return stream, nil
	}
	sc.wg.Add(1)
	return &waitingStream{Stream: stream, done: sync.OnceFunc(sc.wg.Done)}, nil
}

type waitingStream struct {
	streaming.Stream
	done func()
}

// Recv is called in a loop by the receiver, which processes each object before calling Recv again.
// So, once Recv fails, every object received before has been processed.
func (s *waitingStream) Recv() (typeurl.Any, error) {
	a, err := s.Stream.Recv()
	if err != nil {
		s.done()
	}
	return a, err
}
