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

package imagewalker

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	refdocker "github.com/containerd/containerd/reference/docker"
)

type Found struct {
	Image      images.Image
	Req        string // The raw request string. name, short ID, or long ID.
	MatchIndex int    // Begins with 0, up to MatchCount - 1.
	MatchCount int    // 1 on exact match. > 1 on ambiguous match. Never be <= 0.
}

type OnFound func(ctx context.Context, found Found) error

type ImageWalker struct {
	Client  *containerd.Client
	OnFound OnFound
}

// Walk walks images and calls w.OnFound .
// Req is name, short ID, or long ID.
// Returns the number of the found entries.
func (w *ImageWalker) Walk(ctx context.Context, req string) (int, error) {

	var filters []string
	if canonicalRef, err := refdocker.ParseDockerRef(req); err == nil {
		filters = append(filters, fmt.Sprintf("name==%s", canonicalRef.String()))
	}
	//Using Walk API to fetch long ID(s) from contentStore
	cs := w.Client.ContentStore()
	kPrefix := "containerd.io/blobref.index."
	if err := cs.Walk(ctx, func(info content.Info) error {
		for k, v := range info.Labels {
			if strings.HasPrefix(k, kPrefix) {
				filters = append(filters, fmt.Sprintf("target.digest==%s", v))
			}
		}
		return nil
	}, []string{fmt.Sprintf("digest~=^%s.*$", regexp.QuoteMeta(req)),
		fmt.Sprintf("digest~=^sha256:%s.*$", regexp.QuoteMeta(req))}...); err != nil {
		return -1, err
	}

	images, err := w.Client.ImageService().List(ctx, filters...)
	if err != nil {
		return -1, err
	}

	matchCount := len(images)
	for i, img := range images {
		f := Found{
			Image:      img,
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
