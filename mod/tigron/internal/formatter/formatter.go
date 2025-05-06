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

	"golang.org/x/text/width"
)

const (
	maxLineLength = 110
	maxLines      = 50
	kMaxLength    = 7
	spacer        = " "
)

// Table formats a `n x 2` dataset into a series of n rows by 2 columns.
//
//nolint:mnd // Too annoying
func Table(data [][]any, mark string) string {
	var output string

	for _, row := range data {
		key := fmt.Sprintf("%v", row[0])
		value := strings.ReplaceAll(fmt.Sprintf("%v", row[1]), "\t", "  ")

		output += fmt.Sprintf("+%s+\n", strings.Repeat(mark, maxLineLength-2))

		for _, line := range chunk(value, maxLineLength-kMaxLength-7, maxLines) {
			output += fmt.Sprintf(
				"| %s | %s |\n",
				// Keys longer than one line of kMaxLength will be striped to one line
				chunk(key, kMaxLength, 1)[0],
				line,
			)
			key = ""
		}
	}

	output += fmt.Sprintf("+%s+", strings.Repeat(mark, maxLineLength-2))

	return output
}

// chunk does take a string and split it in lines of maxLength size, accounting for characters display width.
func chunk(s string, maxLength, maxLines int) []string {
	chunks := []string{}

	runes := []rune(s)

	size := 0
	start := 0

	for index := range runes {
		var segment string

		switch width.LookupRune(runes[index]).Kind() {
		case width.EastAsianWide, width.EastAsianFullwidth:
			size += 2
		case width.EastAsianAmbiguous, width.Neutral, width.EastAsianHalfwidth, width.EastAsianNarrow:
			size++
		default:
			size++
		}

		switch {
		case runes[index] == '\n':
			// Met a line-break. Pad to size (removing the line break)
			segment = string(runes[start:index])
			segment += strings.Repeat(spacer, maxLength-size+1)
			start = index + 1
			size = 0
		case size == maxLength:
			// Line is full. Add the segment.
			segment = string(runes[start : index+1])
			start = index + 1
			size = 0
		case size > maxLength:
			// Last char was double width. Push it back to next line, and pad with a single space.
			segment = string(runes[start:index]) + spacer
			start = index
			size = 2
		case index == len(runes)-1:
			// End of string. Pad it to size.
			segment = string(runes[start : index+1])
			segment += strings.Repeat(spacer, maxLength-size)
		default:
			continue
		}

		chunks = append(chunks, segment)
	}

	// If really long, preserve the starting first quarter, the trailing three quarters, and inform.
	actualLength := len(chunks)
	if actualLength > maxLines {
		abbreviator := fmt.Sprintf("... %d lines are being ignored...", actualLength-maxLines)
		chunks = append(
			append(chunks[0:maxLines/4], abbreviator+strings.Repeat(spacer, maxLength-len(abbreviator))),
			chunks[actualLength-maxLines*3/4:]...,
		)
		chunks = append(
			[]string{
				fmt.Sprintf("Actual content is %d lines long and has been abbreviated to %d\n", actualLength, maxLines),
				strings.Repeat(spacer, maxLength),
			},
			chunks...,
		)
	} else if actualLength == 0 {
		chunks = []string{strings.Repeat(spacer, maxLength)}
	}

	return chunks
}
