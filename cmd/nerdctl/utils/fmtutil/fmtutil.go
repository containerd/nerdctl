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

package fmtutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"

	"github.com/docker/cli/templates"
	"github.com/spf13/cobra"
)

// Flusher is implemented by text/tabwriter.Writer
type Flusher interface {
	Flush() error
}

func FormatLabels(labels map[string]string) string {
	var res string
	for k, v := range labels {
		s := k + "=" + v
		if res == "" {
			res = s
		} else {
			res += "," + s
		}
	}
	return res
}

// FormatSlice formats the slice with `--format` flag.
//
// --format="" (default): JSON
// --format='{{json .}}': JSON lines
//
// FormatSlice is expected to be only used for `nerdctl OBJECT inspect` commands.
func FormatSlice(cmd *cobra.Command, x []interface{}) error {
	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "":
		b, err := json.MarshalIndent(x, "", "    ")
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(b))
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
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), b.String()+"\n"); err != nil {
				return err
			}
		}
	}
	return nil
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
