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
	"regexp"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/netutil"
)

// hiddenNetworkLabels are nerdctl-internal labels that back network state but are
// not user-facing, so `network ls` output and label filters must not expose them.
var hiddenNetworkLabels = map[string]struct{}{
	labels.NetworkAuxAddresses: {},
}

// visibleNetworkLabels returns a copy of the network's labels with the internal
// keys in hiddenNetworkLabels removed. It returns nil when the input is nil so
// bookkeeping like the aux-address reservation never surfaces in `network ls` or
// matches a `--filter label=` query.
func visibleNetworkLabels(m *map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(*m))
	for k, v := range *m {
		if _, hidden := hiddenNetworkLabels[k]; hidden {
			continue
		}
		out[k] = v
	}
	return out
}

type networkPrintable struct {
	ID     string // empty for non-nerdctl networks
	Name   string
	Labels string
	// TODO: "CreatedAt", "Driver", "IPv6", "Internal", "Scope"
	file string
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

	e, err := netutil.NewCNIEnv(globalOptions.CNIPath, globalOptions.CNINetConfPath, netutil.WithNamespace(options.GOptions.Namespace))
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
			p.Labels = formatter.FormatLabels(visibleNetworkLabels(n.NerdctlLabels))
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
			filter, value, ok := strings.Cut(filter, "=")
			if !ok {
				continue
			}
			switch filter {
			case "name":
				re, err := regexp.Compile(value)
				if err != nil {
					return nil, nil, err
				}
				nameFilterFuncs = append(nameFilterFuncs, func(name string) bool {
					return re.MatchString(name)
				})
			case "label":
				k, v, hasValue := strings.Cut(value, "=")
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
	// Match against the user-visible labels only, so a --filter label= query can
	// neither select on nor be confused by nerdctl-internal keys.
	visible := visibleNetworkLabels(net.NerdctlLabels)
	for _, labelFilterFunc := range labelFilterFuncs {
		if !labelFilterFunc(&visible) {
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
