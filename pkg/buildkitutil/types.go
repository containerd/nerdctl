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
   Portions from https://github.com/moby/buildkit/blob/v0.11.0/client/diskusage.go
   Copyright (C) The BuildKit authors.
   Licensed under the Apache License, Version 2.0
*/

package buildkitutil

import "time"

// UsageInfo is from https://github.com/moby/buildkit/blob/v0.11.0/client/diskusage.go#L12-L25
type UsageInfo struct {
	ID      string `json:"id"`
	Mutable bool   `json:"mutable"`
	InUse   bool   `json:"inUse"`
	Size    int64  `json:"size"`

	CreatedAt   time.Time       `json:"createdAt"`
	LastUsedAt  *time.Time      `json:"lastUsedAt"`
	UsageCount  int             `json:"usageCount"`
	Parents     []string        `json:"parents"`
	Description string          `json:"description"`
	RecordType  UsageRecordType `json:"recordType"`
	Shared      bool            `json:"shared"`
}

// UsageRecordType is from https://github.com/moby/buildkit/blob/v0.11.0/client/diskusage.go#L75
type UsageRecordType string
