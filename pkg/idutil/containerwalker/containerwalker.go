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
	"fmt"
	"regexp"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/labels"
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
	if strings.HasPrefix(req, "k8s://") {
		return -1, fmt.Errorf("specifying \"k8s://...\" form is not supported (Hint: specify ID instead): %q", req)
	}
	filters := []string{
		fmt.Sprintf("labels.%q==%s", labels.Name, req),
		fmt.Sprintf("id~=^%s.*$", regexp.QuoteMeta(req)),
	}

	containers, err := w.Client.Containers(ctx, filters...)
	if err != nil {
		return -1, err
	}

	matchCount := len(containers)
	for i, c := range containers {
		f := Found{
			Container:  c,
			Req:        req,
			MatchIndex: i,
			MatchCount: matchCount,
		}
		if e := w.OnFound(ctx, f); e != nil {
			return -1, e
		}
	}
	return matchCount, nil
}
