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

package netwalker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/containerd/nerdctl/pkg/netutil"
)

type Found struct {
	Network    *netutil.NetworkConfig
	Req        string // The raw request string. name, short ID, or long ID.
	MatchIndex int    // Begins with 0, up to MatchCount - 1.
	MatchCount int    // 1 on exact match. > 1 on ambiguous match. Never be <= 0.
}

type OnFound func(ctx context.Context, found Found) error

type NetworkWalker struct {
	Client  *netutil.CNIEnv
	OnFound OnFound
}

// Walk walks networks and calls w.OnFound .
// Req is name, short ID, or long ID.
// Returns the number of the found entries.
func (w *NetworkWalker) Walk(ctx context.Context, req string) (int, error) {
	longIDExp, err := regexp.Compile(fmt.Sprintf("^sha256:%s.*", regexp.QuoteMeta(req)))
	if err != nil {
		return 0, err
	}

	shortIDExp, err := regexp.Compile(fmt.Sprintf("^%s", regexp.QuoteMeta(req)))
	if err != nil {
		return 0, err
	}

	idFilterF := func(n *netutil.NetworkConfig) bool {
		if n.NerdctlID == nil {
			// External network
			return n.Name == req
		}
		return n.Name == req || longIDExp.Match([]byte(*n.NerdctlID)) || shortIDExp.Match([]byte(*n.NerdctlID))
	}
	networks, err := w.Client.FilterNetworks(idFilterF)
	if err != nil {
		return 0, err
	}

	matchCount := len(networks)

	for i, network := range networks {
		f := Found{
			Network:    network,
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
// `allowSeudoNetwork` allows seudo network (host, none) to be passed to `Walk`, otherwise
// an error is recorded for it.
func (w *NetworkWalker) WalkAll(ctx context.Context, reqs []string, forceAll, allowSeudoNetwork bool) error {
	var errs []string
	for _, req := range reqs {
		if !allowSeudoNetwork && (req == "host" || req == "none") {
			err := fmt.Errorf("pseudo network not allowed: %s", req)
			if !forceAll {
				return err
			}
			errs = append(errs, err.Error())
		} else {
			n, err := w.Walk(ctx, req)
			if err == nil && n == 0 {
				err = fmt.Errorf("no such network: %s", req)
			}
			if err != nil {
				if !forceAll {
					return err
				}
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%d errors:\n%s", len(errs), strings.Join(errs, "\n"))
	}
	return nil
}
