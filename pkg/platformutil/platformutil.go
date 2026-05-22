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
	"slices"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

const ErofsOSFeature = "erofs"

// NewMatchComparerFromOCISpecPlatformSlice returns MatchComparer.
// If platformz is empty, NewMatchComparerFromOCISpecPlatformSlice returns All (not DefaultStrict).
func NewMatchComparerFromOCISpecPlatformSlice(platformz []ocispec.Platform) platforms.MatchComparer {
	if len(platformz) == 0 {
		return platforms.All
	}
	return IgnoreOSFeaturesMatcher(platforms.Ordered(platformz...), ErofsOSFeature)
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
		return IgnoreOSFeaturesMatcher(platforms.DefaultStrict(), ErofsOSFeature), nil
	}
	op, err := NewOCISpecPlatformSlice(false, ss)
	return IgnoreOSFeaturesMatcher(platforms.Ordered(op...), ErofsOSFeature), err
}

// IgnoreOSFeaturesMatcher wraps a MatchComparer and ignores selected os.features
// on candidate platforms before delegating to the wrapped matcher.
func IgnoreOSFeaturesMatcher(mc platforms.MatchComparer, features ...string) platforms.MatchComparer {
	return ignoreOSFeaturesMatcher{
		MatchComparer: mc,
		features:      features,
	}
}

func AppendOSFeatureVariants(platformz []ocispec.Platform, features ...string) []ocispec.Platform {
	if len(platformz) == 0 || len(features) == 0 {
		return platformz
	}
	out := slices.Clone(platformz)
	seen := make(map[string]struct{}, len(platformz)*2)
	for _, p := range out {
		seen[platforms.FormatAll(platforms.Normalize(p))] = struct{}{}
	}
	for _, p := range platformz {
		var added bool
		for _, feature := range features {
			if slices.Contains(p.OSFeatures, feature) {
				continue
			}
			p.OSFeatures = append(p.OSFeatures, feature)
			added = true
		}
		if added {
			p = platforms.Normalize(p)
			key := platforms.FormatAll(p)
			if _, ok := seen[key]; ok {
				continue
			}
			out = append(out, p)
			seen[key] = struct{}{}
		}
	}
	return out
}

type ignoreOSFeaturesMatcher struct {
	platforms.MatchComparer
	features []string
}

func (m ignoreOSFeaturesMatcher) Match(p ocispec.Platform) bool {
	return m.MatchComparer.Match(p) || m.MatchComparer.Match(withoutOSFeatures(p, m.features))
}

func (m ignoreOSFeaturesMatcher) Less(p1, p2 ocispec.Platform) bool {
	p1Match := m.MatchComparer.Match(p1)
	p2Match := m.MatchComparer.Match(p2)
	if p1Match != p2Match {
		return p1Match
	}
	if p1Match {
		return m.MatchComparer.Less(p1, p2)
	}
	return m.MatchComparer.Less(withoutOSFeatures(p1, m.features), withoutOSFeatures(p2, m.features))
}

func withoutOSFeatures(p ocispec.Platform, features []string) ocispec.Platform {
	if !slices.ContainsFunc(p.OSFeatures, func(feature string) bool {
		return slices.Contains(features, feature)
	}) {
		return p
	}
	p.OSFeatures = slices.DeleteFunc(slices.Clone(p.OSFeatures), func(feature string) bool {
		return slices.Contains(features, feature)
	})
	return p
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
