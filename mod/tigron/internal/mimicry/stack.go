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

package mimicry

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/containerd/nerdctl/mod/tigron/internal/formatter"
)

const (
	hyperlinkDecorator = "🔗"
	intoDecorator      = "↪"
)

// Call is used to store information about a call to a function of the mocked struct, including
// arguments, time, and
// frames.
type Call struct {
	Time   time.Time
	Args   []any
	Frames []*Frame
}

// A Frame stores information about a call code-path: file, line number and function name.
type Frame struct {
	File     string
	Function string
	Line     int
}

// String returns an OSC8 hyperlink pointing to the source along with package and function
// information.
// FIXME: we are mixing formatting concerns here.
// FIXME: this is gibberish to read.
func (f *Frame) String() string {
	cwd, _ := os.Getwd()

	rel, err := filepath.Rel(cwd, f.File)
	if err != nil {
		rel = f.File
	}

	spl := strings.Split(f.Function, ".")
	fun := spl[len(spl)-1]
	mod := strings.Join(spl[:len(spl)-1], ".")

	return hyperlinkDecorator + " " + (&formatter.OSC8{
		Location: "file://" + f.File,
		Line:     f.Line,
		Text:     fmt.Sprintf("%s:%d", rel, f.Line),
	}).String() +
		fmt.Sprintf(
			"\n%6s package %q\n",
			intoDecorator,
			mod,
		) +
		fmt.Sprintf(
			"%8s func %s",
			" "+intoDecorator,
			fun,
		)
}

// Excerpt will return the source code content associated with the frame + a few lines around.
func (f *Frame) Excerpt(add int, marker string) string {
	source, err := os.Open(f.File)
	if err != nil {
		return ""
	}

	defer func() {
		_ = source.Close()
	}()

	index := 1
	scanner := bufio.NewScanner(source)

	for ; scanner.Err() == nil && index < f.Line-add; index++ {
		if !scanner.Scan() {
			break
		}

		_ = scanner.Text()
	}

	capt := []string{}

	for ; scanner.Err() == nil && index <= f.Line+add; index++ {
		if !scanner.Scan() {
			break
		}

		line := scanner.Text()
		if index == f.Line {
			line = fmt.Sprintf("%6d %s %s", index, marker, line)
		} else {
			// FIXME: see other similar note. Rune counting is not display-width, so...
			line = fmt.Sprintf("%6d %*s %s", index, utf8.RuneCountInString(marker), "", line)
		}

		capt = append(capt, line)
	}

	return strings.Join(capt, "\n")
}

func isStd(in string) bool {
	return !strings.Contains(in, "/")
}
