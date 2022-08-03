package buildkitutil

import (
	"bufio"
	"bytes"
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
	scanner := bufio.NewScanner(bytes.NewReader(out))
	firstLineParsed := false

	var rows []BuildctlPruneOutputRow
	totalSize := ""
	for scanner.Scan() {
		line := scanner.Text()
		// parse total
		if strings.HasPrefix(line, FinalizerTotal) {
			totalSize = strings.TrimSpace(line[len(FinalizerTotal):])
			break
		}
		// parse header
		if !firstLineParsed {
			_ = tabReader.ParseHeader(line)
			firstLineParsed = true
			continue
		}
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
