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
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/docker/cli/templates"
)

// Flusher is implemented by text/tabwriter.Writer
type Flusher interface {
	Flush() error
}

// tableFormatKey introduces the Docker table formats, e.g. `table {{.Type}}\t{{.Size}}`, which
// render a header and aligned columns rather than the raw output of the template.
const tableFormatKey = "table"

// IsTableFormat reports whether format is a Docker table format: either the bare "table", which
// selects the default columns of a command, or "table " followed by a template.
func IsTableFormat(format string) bool {
	return format == tableFormatKey || strings.HasPrefix(format, tableFormatKey+" ")
}

// ParseTableTemplate parses the template carried by a Docker table format. Like docker/cli, it
// expands the literal `\t` and `\n` a shell would otherwise have to produce itself.
//
// It returns a second template for the header row. A header is rendered by running the very same
// template over the column labels, so a function that transforms a value would rewrite the label
// too and `table {{lower .Type}}` would name the column "type" instead of TYPE. The header template
// therefore replaces those functions by ones leaving their argument alone, as docker/cli does. Only
// `pad` is kept as it is, so that the header stays aligned with its column.
func ParseTableTemplate(format string) (rows, header *template.Template, err error) {
	format = strings.TrimSpace(strings.TrimPrefix(format, tableFormatKey))
	format = strings.ReplaceAll(format, `\t`, "\t")
	format = strings.ReplaceAll(format, `\n`, "\n")

	if rows, err = ParseTemplate(format); err != nil {
		return nil, nil, err
	}
	if header, err = rows.Clone(); err != nil {
		return nil, nil, err
	}
	return rows, header.Funcs(templates.HeaderFunctions), nil
}

// FormatSlice formats the slice with `--format` flag.
//
// --format="" (default): JSON
// --format='{{json .}}': JSON lines
//
// FormatSlice is expected to be only used for `nerdctl OBJECT inspect` commands.
func FormatSlice(format string, writer io.Writer, x []interface{}) error {
	var tmpl *template.Template
	switch format {
	case "":
		// Avoid escaping "<", ">", "&"
		// https://pkg.go.dev/encoding/json
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "    ")
		encoder.SetEscapeHTML(false)
		err := encoder.Encode(x)
		if err != nil {
			return err
		}
		fmt.Fprint(writer, "\n")
	case "raw", "table", "wide":
		return errors.New("unsupported format: \"raw\", \"table\", and \"wide\"")
	default:
		var err error
		tmpl, err = ParseTemplate(format)
		if err != nil {
			return err
		}
		for _, f := range x {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, f); err != nil {
				if _, ok := err.(template.ExecError); ok {
					// FallBack to Raw Format
					if err = tryRawFormat(&b, f, tmpl); err != nil {
						return err
					}
				}
			}
			if _, err = fmt.Fprintln(writer, b.String()); err != nil {
				return err
			}
		}
	}
	return nil
}

// FormatInspectSlice formats inspect results and propagates template errors back
// to the caller so CLI commands can fail with a non-zero exit code.
func FormatInspectSlice(format string, writer io.Writer, x []interface{}) error {
	if len(x) == 0 {
		return nil
	}
	return FormatSlice(format, writer, x)
}

func tryRawFormat(b *bytes.Buffer, f interface{}, tmpl *template.Template) error {
	m, err := json.MarshalIndent(f, "", "    ")
	if err != nil {
		return err
	}

	var raw interface{}
	rdr := bytes.NewReader(m)
	dec := json.NewDecoder(rdr)
	dec.UseNumber()

	if rawErr := dec.Decode(&raw); rawErr != nil {
		return fmt.Errorf("unable to read inspect data: %v", rawErr)
	}

	tmplMissingKey := tmpl.Option("missingkey=error")
	if rawErr := tmplMissingKey.Execute(b, raw); rawErr != nil {
		return fmt.Errorf("template parsing error: %v", rawErr)
	}

	return nil
}

// ParseTemplate wraps github.com/docker/cli/templates.Parse() to allow `json` as an alias of `{{json .}}`.
// ParseTemplate can be removed when https://github.com/docker/cli/pull/3355 gets merged and tagged (Docker 22.XX).
func ParseTemplate(format string) (*template.Template, error) {
	aliases := map[string]string{
		"json": "{{json .}}",
	}
	if alias, ok := aliases[format]; ok {
		format = alias
	}
	return templates.Parse(format)
}
