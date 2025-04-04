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

package formatter

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxLineLength = 110
	maxLines      = 100
	kMaxLength    = 7
)

func chunk(s string, length int) []string {
	var chunks []string

	lines := strings.Split(s, "\n")

	for x := 0; x < maxLines && x < len(lines); x++ {
		line := lines[x]
		if utf8.RuneCountInString(line) < length {
			chunks = append(chunks, line)

			continue
		}

		for index := 0; index < utf8.RuneCountInString(line); index += length {
			end := index + length
			if end > utf8.RuneCountInString(line) {
				end = utf8.RuneCountInString(line)
			}

			chunks = append(chunks, string([]rune(line)[index:end]))
		}
	}

	if len(chunks) == maxLines {
		chunks = append(chunks, "...")
	}

	return chunks
}

// Table formats a `n x 2` dataset into a series of rows.
// FIXME: the problem with full-width emoji is that they are going to eff-up the maths and display
// here...
// Maybe the csv writer could be cheat-used to get the right widths.
//
//nolint:mnd // Too annoying
func Table(data [][]any) string {
	var output string

	for _, row := range data {
		key := fmt.Sprintf("%v", row[0])
		value := strings.ReplaceAll(fmt.Sprintf("%v", row[1]), "\t", "  ")

		output += fmt.Sprintf("+%s+\n", strings.Repeat("-", maxLineLength-2))

		if utf8.RuneCountInString(key) > kMaxLength {
			key = string([]rune(key)[:kMaxLength-3]) + "..."
		}

		for _, line := range chunk(value, maxLineLength-kMaxLength-7) {
			output += fmt.Sprintf(
				"| %-*s | %-*s |\n",
				kMaxLength,
				key,
				maxLineLength-kMaxLength-7,
				line,
			)
			key = ""
		}
	}

	output += fmt.Sprintf("+%s+", strings.Repeat("-", maxLineLength-2))

	return output
}
