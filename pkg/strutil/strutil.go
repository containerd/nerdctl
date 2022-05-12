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

/*
   Portions from https://github.com/moby/moby/blob/v20.10.0-rc2/runconfig/opts/parse.go
   Copyright (C) Docker/Moby authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/moby/moby/blob/v20.10.0-rc2/NOTICE
*/

/*
   Portions from https://github.com/moby/buildkit/blob/v0.9.1/cmd/buildkitd/config.go#L35-L42
   Copyright (C) BuildKit authors.
   Licensed under the Apache License, Version 2.0
*/

package strutil

import (
	"encoding/csv"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/containerd/containerd/errdefs"
)

// ConvertKVStringsToMap is from https://github.com/moby/moby/blob/v20.10.0-rc2/runconfig/opts/parse.go
//
// ConvertKVStringsToMap converts ["key=value"] to {"key":"value"}
func ConvertKVStringsToMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		kv := strings.SplitN(value, "=", 2)
		if len(kv) == 1 {
			result[kv[0]] = ""
		} else {
			result[kv[0]] = kv[1]
		}
	}

	return result
}

// InStringSlice checks whether a string is inside a string slice.
// Comparison is case insensitive.
//
// From https://github.com/containerd/containerd/blob/7c6d710bcfc81a30ac1e8cbb2e6a4c294184f7b7/pkg/cri/util/strings.go#L21-L30
func InStringSlice(ss []string, str string) bool {
	for _, s := range ss {
		if strings.EqualFold(s, str) {
			return true
		}
	}
	return false
}

func DedupeStrSlice(in []string) []string {
	m := make(map[string]struct{})
	var res []string
	for _, s := range in {
		if _, ok := m[s]; !ok {
			res = append(res, s)
			m[s] = struct{}{}
		}
	}
	return res
}

// ParseCSVMap parses a string like "foo=x,bar=y" into a map
func ParseCSVMap(s string) (map[string]string, error) {
	csvR := csv.NewReader(strings.NewReader(s))
	ra, err := csvR.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q: %w", s, err)
	}
	if len(ra) != 1 {
		return nil, fmt.Errorf("expected a single line, got %d lines: %w", len(ra), errdefs.ErrInvalidArgument)
	}
	fields := ra[0]
	m := make(map[string]string)
	for _, field := range fields {
		kv := strings.SplitN(field, "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = kv[1]
		} else {
			m[kv[0]] = ""
		}
	}
	return m, nil
}

func TrimStrSliceRight(base, extra []string) []string {
	for i := 0; i < len(base); i++ {
		if reflect.DeepEqual(base[i:], extra) {
			return base[:i]
		}
	}
	return base
}

func ReverseStrSlice(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[len(in)-i-1] = v
	}
	return out
}

// ParseBoolOrAuto returns (nil, nil) if s is "auto"
// https://github.com/moby/buildkit/blob/v0.9.1/cmd/buildkitd/config.go#L35-L42
func ParseBoolOrAuto(s string) (*bool, error) {
	if s == "" || strings.ToLower(s) == "auto" {
		return nil, nil
	}
	b, err := strconv.ParseBool(s)
	return &b, err
}

type OrderedQuery struct {
	index []string
	data  map[string][]string
}

func NewOrderedQuery() *OrderedQuery {
	return &OrderedQuery{[]string{}, map[string][]string{}}
}

func (orderedQuery *OrderedQuery) Get(key string) string {
	if orderedQuery == nil {
		return ""
	}
	vs := orderedQuery.data[key]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]
}

// Set sets the key to value. It replaces any existing
// values.
func (orderedQuery *OrderedQuery) Set(key, value string) {
	orderedQuery.data[key] = []string{value}
	orderedQuery.index = append(orderedQuery.index, key)
}

// Add adds the value to key. It appends to any existing
// values associated with key.
func (orderedQuery *OrderedQuery) Add(key, value string) {
	orderedQuery.data[key] = append(orderedQuery.data[key], value)
}

// Encode encodes the values into ``URL encoded'' form
// ("bar=baz&foo=quux") sorted by key.
func (orderedQuery *OrderedQuery) Encode() string {
	if orderedQuery == nil {
		return ""
	}
	var buf strings.Builder
	for _, k := range orderedQuery.index {
		vs := orderedQuery.data[k]
		keyEscaped := url.QueryEscape(k)
		for _, v := range vs {
			if buf.Len() > 0 {
				buf.WriteByte('&')
			}
			buf.WriteString(keyEscaped)
			buf.WriteByte('=')
			buf.WriteString(url.QueryEscape(v))
		}
	}
	return buf.String()
}
