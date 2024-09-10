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

package imgutil

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	dockerreference "github.com/distribution/reference"
)

// Filter types supported to filter images.
var (
	FilterBeforeType    = "before"
	FilterSinceType     = "since"
	FilterLabelType     = "label"
	FilterReferenceType = "reference"
	FilterDanglingType  = "dangling"
)

// Filters contains all types of filters to filter images.
type Filters struct {
	Before    []string
	Since     []string
	Labels    map[string]string
	Reference []string
	Dangling  *bool
}

// ParseFilters parse filter strings.
func ParseFilters(filters []string) (*Filters, error) {
	f := &Filters{Labels: make(map[string]string)}
	for _, filter := range filters {
		tempFilterToken := strings.Split(filter, "=")
		switch len(tempFilterToken) {
		case 1:
			return nil, fmt.Errorf("invalid filter %q", filter)
		case 2:
			if tempFilterToken[0] == FilterDanglingType {
				var isDangling bool
				if tempFilterToken[1] == "true" {
					isDangling = true
				} else if tempFilterToken[1] == "false" {
					isDangling = false
				} else {
					return nil, fmt.Errorf("invalid filter %q", filter)
				}
				f.Dangling = &isDangling
			} else if tempFilterToken[0] == FilterBeforeType {
				canonicalRef, err := referenceutil.ParseAny(tempFilterToken[1])
				if err != nil {
					return nil, err
				}

				f.Before = append(f.Before, fmt.Sprintf("name==%s", canonicalRef.String()))
				f.Before = append(f.Before, fmt.Sprintf("name==%s", tempFilterToken[1]))
			} else if tempFilterToken[0] == FilterSinceType {
				canonicalRef, err := referenceutil.ParseAny(tempFilterToken[1])
				if err != nil {
					return nil, err
				}
				f.Since = append(f.Since, fmt.Sprintf("name==%s", canonicalRef.String()))
				f.Since = append(f.Since, fmt.Sprintf("name==%s", tempFilterToken[1]))
			} else if tempFilterToken[0] == FilterLabelType {
				// To support filtering labels by keys.
				f.Labels[tempFilterToken[1]] = ""
			} else if tempFilterToken[0] == FilterReferenceType {
				f.Reference = append(f.Reference, tempFilterToken[1])
			} else {
				return nil, fmt.Errorf("invalid filter %q", filter)
			}
		case 3:
			if tempFilterToken[0] == FilterLabelType {
				f.Labels[tempFilterToken[1]] = tempFilterToken[2]
			} else {
				return nil, fmt.Errorf("invalid filter %q", filter)
			}
		default:
			return nil, fmt.Errorf("invalid filter %q", filter)
		}
	}
	return f, nil
}

// FilterImages returns images in `labelImages` that are created
// before MAX(beforeImages.CreatedAt) and after MIN(sinceImages.CreatedAt).
func FilterImages(labelImages []images.Image, beforeImages []images.Image, sinceImages []images.Image) []images.Image {
	var filteredImages []images.Image
	maxTime := time.Now()
	minTime := time.Date(1970, time.Month(1), 1, 0, 0, 0, 0, time.UTC)
	if len(beforeImages) > 0 {
		maxTime = beforeImages[0].CreatedAt
		for _, value := range beforeImages {
			if value.CreatedAt.After(maxTime) {
				maxTime = value.CreatedAt
			}
		}
	}
	if len(sinceImages) > 0 {
		minTime = sinceImages[0].CreatedAt
		for _, value := range sinceImages {
			if value.CreatedAt.Before(minTime) {
				minTime = value.CreatedAt
			}
		}
	}
	for _, image := range labelImages {
		if image.CreatedAt.After(minTime) && image.CreatedAt.Before(maxTime) {
			filteredImages = append(filteredImages, image)
		}
	}
	return filteredImages
}

// FilterByReference filters images using references given in `filters`.
func FilterByReference(imageList []images.Image, filters []string) ([]images.Image, error) {
	var filteredImageList []images.Image
	log.L.Debug(filters)
	for _, image := range imageList {
		log.L.Debug(image.Name)
		var matches int
		for _, f := range filters {
			var ref dockerreference.Reference
			var err error
			ref, err = dockerreference.ParseAnyReference(image.Name)
			if err != nil {
				return nil, fmt.Errorf("unable to parse image name: %s while filtering by reference because of %s", image.Name, err.Error())
			}

			familiarMatch, err := dockerreference.FamiliarMatch(f, ref)
			if err != nil {
				return nil, err
			}
			regexpMatch, err := regexp.MatchString(f, image.Name)
			if err != nil {
				return nil, err
			}
			if familiarMatch || regexpMatch {
				matches++
			}
		}
		if matches == len(filters) {
			filteredImageList = append(filteredImageList, image)
		}
	}
	return filteredImageList, nil
}

// FilterDangling filters dangling images (or keeps if `dangling` == false).
func FilterDangling(imageList []images.Image, dangling bool) []images.Image {
	var filtered []images.Image
	for _, image := range imageList {
		_, tag := ParseRepoTag(image.Name)

		if dangling && tag == "" {
			filtered = append(filtered, image)
		}
		if !dangling && tag != "" {
			filtered = append(filtered, image)
		}
	}
	return filtered
}

// FilterByLabel filters images based on labels given in `filters`.
func FilterByLabel(ctx context.Context, client *containerd.Client, imageList []images.Image, filters map[string]string) ([]images.Image, error) {
	for lk, lv := range filters {
		var imageLabels []images.Image
		for _, img := range imageList {
			ci := containerd.NewImage(client, img)
			cfg, _, err := ReadImageConfig(ctx, ci)
			if err != nil {
				return nil, err
			}
			if val, ok := cfg.Config.Labels[lk]; ok {
				if val == lv || lv == "" {
					imageLabels = append(imageLabels, img)
				}
			}
		}
		imageList = imageLabels
	}
	return imageList, nil
}
