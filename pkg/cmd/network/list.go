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

package network

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/netutil"
)

type networkPrintable struct {
	ID     string // empty for non-nerdctl networks
	Name   string
	Labels string
	// TODO: "CreatedAt", "Driver", "IPv6", "Internal", "Scope"
	file string `json:"-"`
}

func List(ctx context.Context, options types.NetworkListOptions) error {
	globalOptions := options.GOptions
	quiet := options.Quiet
	format := options.Format
	w := options.Stdout
	filters := options.Filters
	var tmpl *template.Template

	switch format {
	case "", "table", "wide":
		w = tabwriter.NewWriter(w, 4, 8, 4, ' ', 0)
		if !quiet {
			fmt.Fprintln(w, "NETWORK ID\tNAME\tFILE")
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

	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath)
	if err != nil {
		return err
	}
	netConfigs, err := e.NetworkList()
	if err != nil {
		return err
	}

	labelFilterFuncs, nameFilterFuncs, err := getNetworkFilterFuncs(filters)
	if err != nil {
		return err
	}
	if len(filters) > 0 {
		filtered := make([]*netutil.NetworkConfig, 0)
		for _, net := range netConfigs {
			if networkMatchesFilter(net, labelFilterFuncs, nameFilterFuncs) {
				filtered = append(filtered, net)
			}
		}
		netConfigs = filtered
	}

	pp := make([]networkPrintable, len(netConfigs))
	for i, n := range netConfigs {
		p := networkPrintable{
			Name: n.Name,
			file: n.File,
		}
		if n.NerdctlID != nil {
			p.ID = *n.NerdctlID
			if len(p.ID) > 12 {
				p.ID = p.ID[:12]
			}
		}
		if n.NerdctlLabels != nil {
			p.Labels = formatter.FormatLabels(*n.NerdctlLabels)
		}
		pp[i] = p
	}

	// append pseudo networks
	if len(filters) == 0 { // filter a pseudo networks is meanless
		pp = append(pp, []networkPrintable{
			{
				Name: "host",
			},
			{
				Name: "none",
			},
		}...)
	}

	for _, p := range pp {
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintln(w, b.String()); err != nil {
				return err
			}
		} else if quiet {
			if p.ID != "" {
				fmt.Fprintln(w, p.ID)
			}
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.ID, p.Name, p.file)
		}
	}
	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}

func getNetworkFilterFuncs(filters []string) ([]func(*map[string]string) bool, []func(string) bool, error) {
	labelFilterFuncs := make([]func(*map[string]string) bool, 0)
	nameFilterFuncs := make([]func(string) bool, 0)

	for _, filter := range filters {
		if strings.HasPrefix(filter, "name") || strings.HasPrefix(filter, "label") {
			subs := strings.SplitN(filter, "=", 2)
			if len(subs) < 2 {
				continue
			}
			switch subs[0] {
			case "name":
				nameFilterFuncs = append(nameFilterFuncs, func(name string) bool {
					return strings.Contains(name, subs[1])
				})
			case "label":
				v, k, hasValue := "", subs[1], false
				if subs := strings.SplitN(subs[1], "=", 2); len(subs) == 2 {
					hasValue = true
					k, v = subs[0], subs[1]
				}
				labelFilterFuncs = append(labelFilterFuncs, func(labels *map[string]string) bool {
					if labels == nil {
						return false
					}
					val, ok := (*labels)[k]
					if !ok || (hasValue && val != v) {
						return false
					}
					return true
				})
			}
			continue
		}
	}
	return labelFilterFuncs, nameFilterFuncs, nil
}

func networkMatchesFilter(net *netutil.NetworkConfig, labelFilterFuncs []func(*map[string]string) bool, nameFilterFuncs []func(string) bool) bool {
	for _, labelFilterFunc := range labelFilterFuncs {
		if !labelFilterFunc(net.NerdctlLabels) {
			return false
		}
	}
	for _, nameFilterFunc := range nameFilterFuncs {
		if !nameFilterFunc(net.Name) {
			return false
		}
	}

	return true
}
