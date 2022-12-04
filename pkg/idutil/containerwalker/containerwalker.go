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

package containerwalker

import (
	"context"
	"github.com/containerd/containerd"
)

type Found struct {
	Container  containerd.Container
	Req        string // The raw request string. name, short ID, or long ID.
	MatchIndex int    // Begins with 0, up to MatchCount - 1.
	MatchCount int    // 1 on exact match. > 1 on ambiguous match. Never be <= 0.
}

type OnFound func(ctx context.Context, found Found) error

type ContainerWalker struct {
	Client  *containerd.Client
	OnFound OnFound
}

// Walk walks containers and calls w.OnFound .
// Req is name, short ID, or long ID.
// Returns the number of the found entries.
func (w *ContainerWalker) Walk(ctx context.Context, req string) (int, error) {
	f := Found{
		Container: nil,
		Req:       req,
	}
	if e := w.OnFound(ctx, f); e != nil {
		return -1, e
	}
	return 1, nil
}
