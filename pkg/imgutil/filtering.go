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
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	distributionref "github.com/distribution/reference"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images"

	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

// Filter types supported to filter images.
const (
	FilterBeforeType    = "before"
	FilterSinceType     = "since"
	FilterUntilType     = "until"
	FilterLabelType     = "label"
	FilterReferenceType = "reference"
	FilterDanglingType  = "dangling"
)

var (
	errMultipleUntilFilters     = errors.New("more than one until filter provided")
	errNoUntilTimestamp         = errors.New("no until timestamp provided")
	errUnparsableUntilTimestamp = errors.New("unable to parse until timestamp")
)

// Filters contains all types of filters to filter images.
type Filters struct {
	Before    []string
	Since     []string
	Until     string
	Labels    map[string]string
	Reference []string
	Dangling  *bool
}

type Filter func([]images.Image) ([]images.Image, error)

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
			} else if tempFilterToken[0] == FilterUntilType {
				if len(tempFilterToken[0]) == 0 {
					return nil, errNoUntilTimestamp
				} else if len(f.Until) > 0 {
					return nil, errMultipleUntilFilters
				}
				f.Until = tempFilterToken[1]
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

// ApplyFilters applies each filter function in the order provided
// and returns the resulting filtered image list.
func ApplyFilters(imageList []images.Image, filters ...Filter) ([]images.Image, error) {
	var err error
	for _, filter := range filters {
		imageList, err = filter(imageList)
		if err != nil {
			return []images.Image{}, err
		}
	}
	return imageList, nil
}

// FilterByCreatedAt filters an image list to images created before MAX(before.<Image>.CreatedAt)
// and after MIN(since.<Image>.CreatedAt).
func FilterByCreatedAt(ctx context.Context, client *containerd.Client, before []string, since []string) Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		var (
			minTime = time.Date(1970, time.Month(1), 1, 0, 0, 0, 0, time.UTC)
			maxTime = time.Now()
		)

		imageStore := client.ImageService()
		if len(before) > 0 {
			beforeImages, err := imageStore.List(ctx, before...)
			if err != nil {
				return []images.Image{}, err
			}
			maxTime = beforeImages[0].CreatedAt
			for _, image := range beforeImages {
				if image.CreatedAt.After(maxTime) {
					maxTime = image.CreatedAt
				}
			}
		}

		if len(since) > 0 {
			sinceImages, err := imageStore.List(ctx, since...)
			if err != nil {
				return []images.Image{}, err
			}
			minTime = sinceImages[0].CreatedAt
			for _, image := range sinceImages {
				if image.CreatedAt.Before(minTime) {
					minTime = image.CreatedAt
				}
			}
		}

		return filter(imageList, func(i images.Image) (bool, error) {
			return imageCreatedBetween(i, minTime, maxTime), nil
		})
	}
}

// FilterUntil filters images created before the provided timestamp.
func FilterUntil(until string) Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		if len(until) == 0 {
			return []images.Image{}, errNoUntilTimestamp
		}

		var (
			parsedTime time.Time
			err        error
		)

		type parseUntilFunc func(string) (time.Time, error)
		parsingFuncs := []parseUntilFunc{
			func(until string) (time.Time, error) {
				return time.Parse(time.RFC3339, until)
			},
			func(until string) (time.Time, error) {
				return time.Parse(time.RFC3339Nano, until)
			},
			func(until string) (time.Time, error) {
				return time.Parse(time.DateOnly, until)
			},
			func(until string) (time.Time, error) {
				// Go duration strings
				d, err := time.ParseDuration(until)
				if err != nil {
					return time.Time{}, err
				}
				return time.Now().Add(-d), nil
			},
		}

		for _, parse := range parsingFuncs {
			parsedTime, err = parse(until)
			if err != nil {
				continue
			}
			break
		}

		if err != nil {
			return []images.Image{}, errUnparsableUntilTimestamp
		}

		return filter(imageList, func(i images.Image) (bool, error) {
			return imageCreatedBefore(i, parsedTime), nil
		})
	}
}

// FilterByLabel filters an image list based on labels applied to the image's config specification for the platform.
// Any matching label will include the image in the list.
func FilterByLabel(ctx context.Context, client *containerd.Client, labels map[string]string) Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		return filter(imageList, func(i images.Image) (bool, error) {
			clientImage := containerd.NewImage(client, i)
			imageCfg, _, err := ReadImageConfig(ctx, clientImage)
			if err != nil {
				return false, err
			}
			return matchesAllLabels(imageCfg.Config.Labels, labels), nil
		})
	}
}

// FilterByReference filters an image list based on <image:tag>
// matching the provided reference patterns
func FilterByReference(referencePatterns []string) Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		return filter(imageList, func(i images.Image) (bool, error) {
			return matchesReferences(i, referencePatterns)
		})
	}
}

// FilterDanglingImages filters an image list for dangling (untagged) images.
func FilterDanglingImages() Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		return filter(imageList, func(i images.Image) (bool, error) {
			return isDangling(i), nil
		})
	}
}

// FilterTaggedImages filters an image list for tagged images.
func FilterTaggedImages() Filter {
	return func(imageList []images.Image) ([]images.Image, error) {
		return filter(imageList, func(i images.Image) (bool, error) {
			return !isDangling(i), nil
		})
	}
}

func filter[T any](items []T, f func(item T) (bool, error)) ([]T, error) {
	filteredItems := make([]T, 0, len(items))
	for _, item := range items {
		ok, err := f(item)
		if err != nil {
			return []T{}, err
		} else if ok {
			filteredItems = append(filteredItems, item)
		}
	}
	return filteredItems, nil
}

func imageCreatedBetween(image images.Image, min time.Time, max time.Time) bool {
	return image.CreatedAt.After(min) && image.CreatedAt.Before(max)
}

func imageCreatedBefore(image images.Image, max time.Time) bool {
	return image.CreatedAt.Before(max)
}

func matchesAllLabels(imageCfgLabels map[string]string, filterLabels map[string]string) bool {
	var matches int
	for lk, lv := range filterLabels {
		if val, ok := imageCfgLabels[lk]; ok {
			if val == lv || lv == "" {
				matches++
			}
		}
	}
	return matches == len(filterLabels)
}

func matchesReferences(image images.Image, referencePatterns []string) (bool, error) {
	var matches int

	reference, err := distributionref.ParseAnyReference(image.Name)
	if err != nil {
		return false, err
	}

	for _, pattern := range referencePatterns {
		familiarMatch, err := distributionref.FamiliarMatch(pattern, reference)
		if err != nil {
			return false, err
		}

		regexpMatch, err := regexp.MatchString(pattern, image.Name)
		if err != nil {
			return false, err
		}

		if familiarMatch || regexpMatch {
			matches++
		}
	}

	return matches == len(referencePatterns), nil
}

func isDangling(image images.Image) bool {
	_, tag := ParseRepoTag(image.Name)
	return tag == ""
}
