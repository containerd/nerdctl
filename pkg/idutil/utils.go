/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package idutil

import (
	"context"
	"strings"

	"github.com/containerd/containerd"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type listIDFunc func(context.Context) ([]string, error)

func listContainerIDs(client *containerd.Client, filters ...string) listIDFunc {
	return func(ctx context.Context) ([]string, error) {
		cntrs, err := client.Containers(ctx, filters...)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to list containers (filters=%+v)", filters)
		}

		ids := make([]string, 0, len(cntrs))
		for _, ctr := range cntrs {
			ids = append(ids, ctr.ID())
		}
		return ids, nil
	}
}

func shortIDToLongIDsFunc(ctx context.Context, listFn listIDFunc, upToTwo bool) (func(string) []string, error) {
	allIDs, err := listFn(ctx)
	if err != nil {
		return nil, err
	}

	return func(shortID string) []string {
		res := make([]string, 0, 2)

		for _, id := range allIDs {
			if strings.HasPrefix(id, shortID) {
				res = append(res, id)

				if upToTwo && len(res) >= 2 {
					break
				}
			}
		}
		return res
	}, nil
}

// WalkFunc defines the callback for a container or image walk.
//
// NOTE: ID begins with shortID.
type WalkFunc func(ctx context.Context, client *containerd.Client, shortID, ID string) error

// WalkContainers will call the provided function for each containers which
// match the provided shortIDs.
func WalkContainers(ctx context.Context, client *containerd.Client, shortIDs []string, fn WalkFunc) error {
	const idLength = 64

	idMapFunc, err := shortIDToLongIDsFunc(ctx, listContainerIDs(client), true)
	if err != nil {
		return err
	}

	for idx, id := range shortIDs {
		if len(id) < idLength {
			found := idMapFunc(id)

			if got := len(found); got > 1 {
				logrus.Errorf("Ambiguous container ID: %s", id)
				continue
			} else if got == 1 {
				id = found[0]
			}
		}
		if err := fn(ctx, client, shortIDs[idx], id); err != nil {
			return err
		}
	}
	return nil
}
