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

package platformutil

import (
	"fmt"

	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// NewMatchComparerFromOCISpecPlatformSlice returns MatchComparer.
// If platformz is empty, NewMatchComparerFromOCISpecPlatformSlice returns All (not DefaultStrict).
func NewMatchComparerFromOCISpecPlatformSlice(platformz []ocispec.Platform) platforms.MatchComparer {
	if len(platformz) == 0 {
		return platforms.All
	}
	return platforms.Ordered(platformz...)
}

// NewMatchComparer returns MatchComparer.
// If all is true, NewMatchComparer always returns All, regardless to the value of ss.
// If all is false and ss is empty, NewMatchComparer returns DefaultStrict (not Default).
// Otherwise NewMatchComparer returns Ordered MatchComparer.
func NewMatchComparer(all bool, ss []string) (platforms.MatchComparer, error) {
	if all {
		return platforms.All, nil
	}
	if len(ss) == 0 {
		// return DefaultStrict, not Default
		return platforms.DefaultStrict(), nil
	}
	op, err := NewOCISpecPlatformSlice(false, ss)
	return platforms.Ordered(op...), err
}

// NewOCISpecPlatformSlice returns a slice of ocispec.Platform
// If all is true, NewOCISpecPlatformSlice always returns an empty slice, regardless to the value of ss.
// If all is false and ss is empty, NewOCISpecPlatformSlice returns DefaultSpec.
// Otherwise NewOCISpecPlatformSlice returns the slice that correspond to ss.
func NewOCISpecPlatformSlice(all bool, ss []string) ([]ocispec.Platform, error) {
	if all {
		return nil, nil
	}
	if dss := strutil.DedupeStrSlice(ss); len(dss) > 0 {
		var op []ocispec.Platform
		for _, s := range dss {
			p, err := platforms.Parse(s)
			if err != nil {
				return nil, fmt.Errorf("invalid platform: %q", s)
			}
			op = append(op, p)
		}
		return op, nil
	}
	return []ocispec.Platform{platforms.DefaultSpec()}, nil
}

func NormalizeString(s string) (string, error) {
	if s == "" {
		return platforms.DefaultString(), nil
	}
	parsed, err := platforms.Parse(s)
	if err != nil {
		return "", err
	}
	normalized := platforms.Normalize(parsed)
	return platforms.Format(normalized), nil
}
