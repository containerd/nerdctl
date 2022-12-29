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

package main

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"

	"github.com/containerd/containerd/pkg/progress"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/inspecttypes/native"
	"github.com/sirupsen/logrus"

	"github.com/spf13/cobra"
)

func newVolumeLsCommand() *cobra.Command {
	volumeLsCommand := &cobra.Command{
		Use:           "ls",
		Aliases:       []string{"list"},
		Short:         "List volumes",
		RunE:          volumeLsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	volumeLsCommand.Flags().BoolP("quiet", "q", false, "Only display volume names")
	// Alias "-f" is reserved for "--filter"
	volumeLsCommand.Flags().String("format", "", "Format the output using the given go template")
	volumeLsCommand.Flags().BoolP("size", "s", false, "Display the disk usage of volumes. Can be slow with volumes having loads of directories.")
	volumeLsCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json", "table", "wide"}, cobra.ShellCompDirectiveNoFileComp
	})
	volumeLsCommand.Flags().StringSliceP("filter", "f", []string{}, "Filter matches volumes based on given conditions")
	return volumeLsCommand
}

type volumePrintable struct {
	Driver     string
	Labels     string
	Mountpoint string
	Name       string
	Scope      string
	Size       string
	// TODO: "Links"
}

func volumeLsAction(cmd *cobra.Command, args []string) error {
	quiet, err := cmd.Flags().GetBool("quiet")
	if err != nil {
		return err
	}
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return err
	}
	if quiet && volumeSize {
		logrus.Warn("cannot use --size and --quiet together, ignoring --size")
		volumeSize = false
	}
	filters, err := cmd.Flags().GetStringSlice("filter")
	if err != nil {
		return err
	}
	labelFilterFuncs, nameFilterFuncs, sizeFilterFuncs, isFilter, err := getVolumeFilterFuncs(filters)
	if err != nil {
		return err
	}
	if len(sizeFilterFuncs) > 0 && quiet {
		logrus.Warn("cannot use --filter=size and --quiet together, ignoring --filter=size")
		sizeFilterFuncs = nil
	}
	if len(sizeFilterFuncs) > 0 && !volumeSize {
		logrus.Warn("should use --filter=size and --size together")
		cmd.Flags().Set("size", "true")
		volumeSize = true
	}
	w := cmd.OutOrStdout()
	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "", "table", "wide":
		w = tabwriter.NewWriter(cmd.OutOrStdout(), 4, 8, 4, ' ', 0)
		if !quiet {
			if volumeSize {
				fmt.Fprintln(w, "VOLUME NAME\tDIRECTORY\tSIZE")
			} else {
				fmt.Fprintln(w, "VOLUME NAME\tDIRECTORY")
			}
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

	vols, err := getVolumes(cmd)
	if err != nil {
		return err
	}

	for _, v := range vols {
		if isFilter && !volumeMatchesFilter(v, labelFilterFuncs, nameFilterFuncs, sizeFilterFuncs) {
			continue
		}
		p := volumePrintable{
			Driver:     "local",
			Labels:     "",
			Mountpoint: v.Mountpoint,
			Name:       v.Name,
			Scope:      "local",
		}
		if v.Labels != nil {
			p.Labels = formatter.FormatLabels(*v.Labels)
		}
		if volumeSize {
			p.Size = progress.Bytes(v.Size).String()
		}
		if tmpl != nil {
			var b bytes.Buffer
			if err := tmpl.Execute(&b, p); err != nil {
				return err
			}
			if _, err = fmt.Fprintf(w, b.String()+"\n"); err != nil {
				return err
			}
		} else if quiet {
			fmt.Fprintln(w, p.Name)
		} else if volumeSize {
			fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.Mountpoint, p.Size)
		} else {
			fmt.Fprintf(w, "%s\t%s\n", p.Name, p.Mountpoint)
		}
	}
	if f, ok := w.(formatter.Flusher); ok {
		return f.Flush()
	}
	return nil
}

func getVolumes(cmd *cobra.Command) (map[string]native.Volume, error) {
	volStore, err := getVolumeStore(cmd)
	if err != nil {
		return nil, err
	}
	volumeSize, err := cmd.Flags().GetBool("size")
	if err != nil {
		return nil, err
	}
	return volStore.List(volumeSize)
}

func getVolumeFilterFuncs(filters []string) ([]func(*map[string]string) bool, []func(string) bool, []func(int64) bool, bool, error) {
	isFilter := len(filters) > 0
	labelFilterFuncs := make([]func(*map[string]string) bool, 0)
	nameFilterFuncs := make([]func(string) bool, 0)
	sizeFilterFuncs := make([]func(int64) bool, 0)
	sizeOperators := []struct {
		Operand string
		Compare func(int64, int64) bool
	}{
		{">=", func(size, volumeSize int64) bool {
			return volumeSize >= size
		}},
		{"<=", func(size, volumeSize int64) bool {
			return volumeSize <= size
		}},
		{">", func(size, volumeSize int64) bool {
			return volumeSize > size
		}},
		{"<", func(size, volumeSize int64) bool {
			return volumeSize < size
		}},
		{"=", func(size, volumeSize int64) bool {
			return volumeSize == size
		}},
	}
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
		if strings.HasPrefix(filter, "size") {
			for _, sizeOperator := range sizeOperators {
				if subs := strings.SplitN(filter, sizeOperator.Operand, 2); len(subs) == 2 {
					v, err := strconv.Atoi(subs[1])
					if err != nil {
						return nil, nil, nil, false, err
					}
					sizeFilterFuncs = append(sizeFilterFuncs, func(size int64) bool {
						return sizeOperator.Compare(int64(v), size)
					})
					break
				}
			}
			continue
		}
	}
	return labelFilterFuncs, nameFilterFuncs, sizeFilterFuncs, isFilter, nil
}

func volumeMatchesFilter(vol native.Volume, labelFilterFuncs []func(*map[string]string) bool, nameFilterFuncs []func(string) bool, sizeFilterFuncs []func(int64) bool) bool {
	for _, labelFilterFunc := range labelFilterFuncs {
		if !labelFilterFunc(vol.Labels) {
			return false
		}
	}
	for _, nameFilterFunc := range nameFilterFuncs {
		if !nameFilterFunc(vol.Name) {
			return false
		}
	}
	for _, sizeFilterFunc := range sizeFilterFuncs {
		if !sizeFilterFunc(vol.Size) {
			return false
		}
	}
	return true
}
