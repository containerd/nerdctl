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
   Portions from https://github.com/docker/cli/blob/v20.10.9/cli/command/image/build/context.go
   Copyright (C) Docker authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/docker/cli/blob/v20.10.9/NOTICE
*/

package buildkitutil

import (
	"fmt"
	"github.com/containerd/nerdctl/pkg/tabutil"
	"strings"
)

type BuildctlPruneOutput struct {
	TotalSize string
	Rows      []BuildctlPruneOutputRow
}

type BuildctlPruneOutputRow struct {
	ID           string
	Reclaimable  string
	Size         string
	LastAccessed string
}

const HeaderID = "ID"
const HeaderReclaimable = "RECLAIMABLE"
const HeaderSize = "SIZE"
const HeaderLastAccessed = "LAST ACCESSED"
const FinalizerTotal = "Total:"

func ParseBuildctlPruneTableOutput(out []byte) (*BuildctlPruneOutput, error) {
	tabReader := tabutil.NewReader(fmt.Sprintf("%s\t%s\t%s\t%s", HeaderID, HeaderReclaimable, HeaderSize, HeaderLastAccessed))
	lines := strings.Split(string(out), "\n")

	totalSize, err := parseTotalSize(lines[len(lines)-2])
	if err != nil {
		return nil, err
	}

	if len(lines) == 2 {
		return &BuildctlPruneOutput{
			TotalSize: totalSize,
			Rows:      nil,
		}, nil
	}

	if err := tabReader.ParseHeader(lines[0]); err != nil {
		return nil, err
	}

	var rows []BuildctlPruneOutputRow

	for _, line := range lines[1 : len(lines)-2] {
		// best effort parse row
		id, _ := tabReader.ReadRow(line, HeaderID)
		reclaimable, _ := tabReader.ReadRow(line, HeaderReclaimable)
		size, _ := tabReader.ReadRow(line, HeaderSize)
		lastAccessed, _ := tabReader.ReadRow(line, HeaderLastAccessed)
		rows = append(rows, BuildctlPruneOutputRow{
			ID:           id,
			Reclaimable:  reclaimable,
			Size:         size,
			LastAccessed: lastAccessed,
		})
	}

	return &BuildctlPruneOutput{
		TotalSize: totalSize,
		Rows:      rows,
	}, nil
}

func parseTotalSize(line string) (string, error) {
	if strings.HasPrefix(line, FinalizerTotal) {
		return strings.TrimSpace(line[len(FinalizerTotal):]), nil
	}
	return "", fmt.Errorf("parse total size from buildctl prune command ouput, unexpected line, does not contains total size: %s", line)
}
