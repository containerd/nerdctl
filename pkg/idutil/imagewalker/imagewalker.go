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

type OnFound func(ctx context.Context, found Found) error

/*
In order to resolve the issue with OnFoundCriRm, the same imageId under
k8s.io is showing multiple results: repo:tag, repo:digest, configID. We expect
to display only repo:tag, consistent with other namespaces and CRI.
e.g.

	nerdctl -n k8s.io images
	REPOSITORY    TAG       IMAGE ID        CREATED        PLATFORM       SIZE         BLOB SIZE
	centos        7         be65f488b776    3 hours ago    linux/amd64    211.5 MiB    72.6 MiB
	centos        <none>    be65f488b776    3 hours ago    linux/amd64    211.5 MiB    72.6 MiB
	<none>        <none>    be65f488b776    3 hours ago    linux/amd64    211.5 MiB    72.6 MiB

The boolean value will return true only when the repo:tag is successfully
deleted for each image. Once all repo:tag entries are deleted, it is necessary
to clean up the remaining repo:digest and configID.
*/
type OnFoundCriRm func(ctx context.Context, found Found) (bool, error)

type ImageWalker struct {
	Client       *containerd.Client
	OnFound      OnFound
	OnFoundCriRm OnFoundCriRm
}

// Walk walks images and calls w.OnFound .
// Req is name, short ID, or long ID.
// Returns the number of the found entries.
func (w *ImageWalker) Walk(ctx context.Context, req string) (int, error) {
	var filters []string
	var parsedReferenceStr string

	parsedReference, err := referenceutil.Parse(req)
	if err == nil {
		parsedReferenceStr = parsedReference.String()
		filters = append(filters, fmt.Sprintf("name==%s", parsedReferenceStr))
	}
	filters = append(filters,
		fmt.Sprintf("name==%s", req),
		fmt.Sprintf("target.digest~=^sha256:%s.*$", regexp.QuoteMeta(req)),
		fmt.Sprintf("target.digest~=^%s.*$", regexp.QuoteMeta(req)),
	)

	images, err := w.Client.ImageService().List(ctx, filters...)
	if err != nil {
		return -1, err
	}

	matchCount := len(images)
	// to handle the `rmi -f` case where returned images are different but
	// have the same short prefix.
	uniqueImages := make(map[digest.Digest]bool)
	nameMatchIndex := -1
	for i, image := range images {
		uniqueImages[image.Target.Digest] = true
		// to get target image index for `nerdctl rmi <short digest ids of another images>`.
		if (parsedReferenceStr != "" && image.Name == parsedReferenceStr) || image.Name == req {
			nameMatchIndex = i
		}
	}

	for i, img := range images {
		f := Found{
			Image:          img,
			Req:            req,
			MatchIndex:     i,
			MatchCount:     matchCount,
			UniqueImages:   len(uniqueImages),
			NameMatchIndex: nameMatchIndex,
		}
		if e := w.OnFound(ctx, f); e != nil {
			return -1, e
		}
	}
	return matchCount, nil
}

// WalkCriRm walks images and calls w.OnFoundCriRm .
// Only effective when in the k8s.io namespace and kube-hide-dupe is enabled.
// The WalkCriRm deletes non-repo:tag items such as repo:digest when in the no-other-repo:tag scenario.
func (w *ImageWalker) WalkCriRm(ctx context.Context, req string) (int, error) {
	var filters []string
	var parsedReferenceStr, repo string
	var imageTag, imagesRepo []images.Image
	var tagNum int

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

	// to handle the `rmi -f` case where returned images are different but
	// have the same short prefix.
	uniqueImages := make(map[digest.Digest]bool)
	nameMatchIndex := -1

	//Distinguish between tag and non-tag
	for _, img := range images {
		ref := img.Name
		parsed, err := referenceutil.Parse(ref)
		if err != nil {
			continue
		}
		if parsed.Tag != "" {
			imageTag = append(imageTag, img)
			tagNum++
			uniqueImages[img.Target.Digest] = true
			// to get target image index for `nerdctl rmi <short digest ids of another images>`.
			if (parsedReferenceStr != "" && img.Name == parsedReferenceStr) || img.Name == req {
				nameMatchIndex = len(imageTag) - 1
			}
		} else {
			imagesRepo = append(imagesRepo, img)
		}
	}

	matchCount := len(imageTag)
	if matchCount < 1 && len(imagesRepo) > 0 {
		matchCount = 1
	}

	for i, img := range imageTag {
		f := Found{
			Image:          img,
			Req:            req,
			MatchIndex:     i,
			MatchCount:     matchCount,
			UniqueImages:   len(uniqueImages),
			NameMatchIndex: nameMatchIndex,
		}
		if ok, e := w.OnFoundCriRm(ctx, f); e != nil {
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
				UniqueImages:   1,
				NameMatchIndex: -1,
			}
			if _, e := w.OnFoundCriRm(ctx, f); e != nil {
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
