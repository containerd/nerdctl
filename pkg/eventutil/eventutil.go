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

package eventutil

import (
	"sync"

	"github.com/containerd/containerd/events"
)

type eventHandler struct {
	handlers map[string]func(events.Envelope)
	mu       sync.Mutex
}

// InitEventHandler initializes and returns an eventHandler
func InitEventHandler() *eventHandler { //nolint:revive
	return &eventHandler{handlers: make(map[string]func(events.Envelope))}
}

func (w *eventHandler) Handle(action string, h func(events.Envelope)) {
	w.mu.Lock()
	w.handlers[action] = h
	w.mu.Unlock()
}

// Watch ranges over the passed in event chan and processes the events based on the
// handlers created for a given action.
// To stop watching, close the event chan.
func (w *eventHandler) Watch(c <-chan *events.Envelope) {
	for e := range c {
		w.mu.Lock()
		h, exists := w.handlers[e.Topic]
		w.mu.Unlock()
		if !exists {
			continue
		}
		go h(*e)
	}
}
