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

package apparmor

import (
	"bytes"
	"errors"
	"fmt"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/apparmorutil"
	"github.com/containerd/nerdctl/pkg/formatter"
)

func Ls(options *types.ApparmorLsCommandOptions) error {
	quiet := options.Quiet
	w := options.Writer
	var tmpl *template.Template
	format := options.Format
	switch format {
	case "", "table", "wide":
		w = tabwriter.NewWriter(options.Writer, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "NAME\tMODE")
		}
	case "raw":
		return errors.New("unsupported format: \"raw\"")
	default:
		if quiet {
			return errors.New("format and quiet must not be specified together")
		}
		var err error
		tmpl, err = formatter.ParseTemplate(format)
		if err != nil {
			return err
		}
	}

	profiles, err := apparmorutil.Profiles()
	if err != nil {
		return err
	}

	for _, f := range profiles {
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, f); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			fmt.Fprintln(w, f.Name)
		} else {
			fmt.Fprintf(w, "%s\t%s\n", f.Name, f.Mode)
		}
	}
	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}
