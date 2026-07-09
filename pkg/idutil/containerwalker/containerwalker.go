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

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/containerd/nerdctl/v2/pkg/labels"
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

// WalkAll calls `Walk` for each req in `reqs`.
//
// It can be used when the matchCount is not important (e.g., only care if there
// is any error or if matchCount == 0 (not found error) when walking all reqs).
// If `forceAll`, it calls `Walk` on every req
// and return all errors joined by `\n`. If not `forceAll`, it returns the first error
// encountered while calling `Walk`.
func (w *ContainerWalker) WalkAll(ctx context.Context, reqs []string, forceAll bool) error {
	var errs []string
	for _, req := range reqs {
		n, err := w.Walk(ctx, req)
		if err == nil && n == 0 {
			err = fmt.Errorf("no such container: %s", req)
		}
		if err != nil {
			if !forceAll {
				return err
			}
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d errors:\n%s", len(errs), strings.Join(errs, "\n"))
	}
	return nil
}
