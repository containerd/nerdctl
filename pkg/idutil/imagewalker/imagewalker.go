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

	"github.com/distribution/reference"
	"github.com/opencontainers/go-digest"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

type Found struct {
	Image          images.Image
	Req            string // The raw request string. name, short ID, or long ID.
	MatchIndex     int    // Begins with 0, up to MatchCount - 1.
	MatchCount     int    // 1 on exact match. > 1 on ambiguous match. Never be <= 0.
	UniqueImages   int    // Number of unique images in all found images.
	NameMatchIndex int    // Image index with a name matching the argument for `nerdctl rmi`.
}

type OnFound func(ctx context.Context, found Found) (error, bool)

type ImageWalker struct {
	Client  *containerd.Client
	OnFound OnFound
}

// Walk walks images and calls w.OnFound .
// Req is name, short ID, or long ID.
// Returns the number of the found entries.
func (w *ImageWalker) Walk(ctx context.Context, req string) (int, error) {
	var filters []string
	var parsedReferenceStr string
	var imagesTag, imagesRepo []images.Image
	var tagNum int
	var repo string

	parsedReference, err := referenceutil.Parse(req)
	if err == nil {
		parsedReferenceStr = parsedReference.String()
		filters = append(filters, fmt.Sprintf("name==%s", parsedReferenceStr))
	}
	//Get the image ID , if reg == imageTag use
	image, err := w.Client.GetImage(ctx, parsedReferenceStr)
	if err != nil {
		repo = req
	} else {
		repo = strings.Split(image.Target().Digest.String(), ":")[1][:12]
	}

	filters = append(filters,
		fmt.Sprintf("name==%s", req),
		fmt.Sprintf("target.digest~=^sha256:%s.*$", regexp.QuoteMeta(repo)),
		fmt.Sprintf("target.digest~=^%s.*$", regexp.QuoteMeta(repo)),
	)

	images, err := w.Client.ImageService().List(ctx, filters...)
	if err != nil {
		return -1, err
	}

	//Distinguish between tag and non-tag
	for _, ima := range images {
		ref := ima.Name
		parsed, err := reference.ParseAnyReference(ref)
		if err != nil {
			continue
		}
		switch parsed.(type) {
		case reference.Canonical, reference.Digested:
			imagesRepo = append(imagesRepo, ima)
		case reference.Tagged:
			imagesTag = append(imagesTag, ima)
			tagNum++
		}
	}

	matchCount := len(imagesTag)
	// to handle the `rmi -f` case where returned images are different but
	// have the same short prefix.
	uniqueImages := make(map[digest.Digest]bool)
	nameMatchIndex := -1
	for i, image := range imagesTag {
		uniqueImages[image.Target.Digest] = true
		// to get target image index for `nerdctl rmi <short digest ids of another images>`.
		if (parsedReferenceStr != "" && image.Name == parsedReferenceStr) || image.Name == req {
			nameMatchIndex = i
		}
	}

	//The matchCount count is only required if it is passed in as an image ID
	if nameMatchIndex != -1 || matchCount < 1 {
		matchCount = 1
	}

	for i, img := range imagesTag {
		f := Found{
			Image:          img,
			Req:            req,
			MatchIndex:     i,
			MatchCount:     matchCount,
			UniqueImages:   len(uniqueImages),
			NameMatchIndex: nameMatchIndex,
		}
		e, ok := w.OnFound(ctx, f)
		if e != nil {
			return -1, e
		} else if ok {
			tagNum = tagNum - 1
		}
	}
	//If the corresponding imageTag does not exist, delete the repoDigests
	if tagNum == 0 {
		for i, img := range imagesRepo {
			f := Found{
				Image:          img,
				Req:            req,
				MatchIndex:     i,
				MatchCount:     1,
				UniqueImages:   len(uniqueImages),
				NameMatchIndex: -1,
			}
			if e, _ := w.OnFound(ctx, f); e != nil {
				return -1, e
			}
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
func (w *ImageWalker) WalkAll(ctx context.Context, reqs []string, forceAll bool) error {
	var errs []string
	for _, req := range reqs {
		n, err := w.Walk(ctx, req)
		if err == nil && n == 0 {
			err = fmt.Errorf("no such image: %s", req)
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
