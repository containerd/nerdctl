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
			return false
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
