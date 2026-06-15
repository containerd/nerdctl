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

package pipetagger

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io"
	"strings"

	"github.com/fatih/color"
)

func ChooseColorAttrs(tag string) []color.Attribute {
	hasher := fnv.New32()
	hasher.Write([]byte(tag))
	tagHash := int(hasher.Sum32())

	fgCandidates := []color.Attribute{
		color.FgBlack,
		color.FgRed,
		color.FgGreen,
		color.FgYellow,
		color.FgBlue,
		color.FgMagenta,
		color.FgCyan,
		color.FgWhite,
		color.FgHiBlack,
		color.FgHiRed,
		color.FgHiGreen,
		color.FgHiYellow,
		color.FgHiBlue,
		color.FgHiMagenta,
		color.FgHiCyan,
		color.FgHiWhite,
	}
	fgAttr := fgCandidates[tagHash%len(fgCandidates)]

	attrs := []color.Attribute{
		fgAttr,
	}

	switch fgAttr {
	case color.FgBlack:
		attrs = append(attrs, color.BgWhite)
	case color.FgWhite:
		attrs = append(attrs, color.BgBlack)
	case color.FgHiBlack:
		attrs = append(attrs, color.BgHiWhite)
	case color.FgHiWhite:
		attrs = append(attrs, color.BgHiBlack)
	}

	return attrs
}

// New create a PipeTagger.
// Set width = -1 to disable tagging.
func New(w io.Writer, r io.Reader, tag string, width int, noColor bool) *PipeTagger {
	var attrs []color.Attribute
	if !noColor {
		attrs = ChooseColorAttrs(tag)
	}
	return &PipeTagger{
		w:     w,
		r:     r,
		tag:   tag,
		width: width,
		color: color.New(attrs...),
	}
}

type PipeTagger struct {
	w     io.Writer
	r     io.Reader
	tag   string
	width int
	color *color.Color
}

func (x *PipeTagger) Run() error {
	scanner := bufio.NewScanner(x.r)
	for scanner.Scan() {
		line := scanner.Text()
		if x.width < 0 {
			fmt.Fprintln(x.w, line)
		} else {
			fmt.Fprintf(x.w, "%s%s|%s\n",
				x.color.Sprint(x.tag),
				strings.Repeat(" ", x.width-len(x.tag)),
				line,
			)
		}
	}
	return scanner.Err()
}
