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

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/cmd/nerdctl/helpers"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/labels"
)

type projectListOptions struct {
	all     bool
	quiet   bool
	format  string
	filters []*regexp.Regexp
}

type projectContainer struct {
	project     string
	state       string
	configFiles string
}

type projectSummary struct {
	Name        string
	Status      string
	ConfigFiles string
}

func lsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ls [flags]",
		Short:         "List running Compose projects",
		Args:          cobra.NoArgs,
		RunE:          lsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().BoolP("all", "a", false, "Show stopped Compose projects as well")
	cmd.Flags().BoolP("quiet", "q", false, "Only display project names")
	cmd.Flags().String("format", "table", "Format the output. Supported values: [table|json]")
	cmd.Flags().StringArray("filter", nil, "Filter projects by name (name=REGEXP)")
	return cmd
}

func readProjectListOptions(cmd *cobra.Command) (projectListOptions, error) {
	var opts projectListOptions
	var err error
	opts.all, err = cmd.Flags().GetBool("all")
	if err != nil {
		return opts, err
	}
	opts.quiet, err = cmd.Flags().GetBool("quiet")
	if err != nil {
		return opts, err
	}
	opts.format, err = cmd.Flags().GetString("format")
	if err != nil {
		return opts, err
	}
	if opts.format != "table" && opts.format != "json" {
		return opts, fmt.Errorf("unsupported format %q, supported formats are: [table|json]", opts.format)
	}
	filters, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		return opts, err
	}
	for _, filter := range filters {
		key, value, ok := strings.Cut(filter, "=")
		if !ok || key != "name" || value == "" {
			return opts, fmt.Errorf("invalid project filter %q: expected name=REGEXP", filter)
		}
		pattern, err := regexp.Compile(value)
		if err != nil {
			return opts, fmt.Errorf("invalid project name filter %q: %w", value, err)
		}
		opts.filters = append(opts.filters, pattern)
	}
	return opts, nil
}

func lsAction(cmd *cobra.Command, _ []string) error {
	opts, err := readProjectListOptions(cmd)
	if err != nil {
		return err
	}
	globalOptions, err := helpers.ProcessRootCmdFlags(cmd)
	if err != nil {
		return err
	}
	client, ctx, cancel, err := clientutil.NewClient(cmd.Context(), globalOptions.Namespace, globalOptions.Address)
	if err != nil {
		return err
	}
	defer cancel()
	defer client.Close()

	// Listing projects must work without loading a compose file or creating
	// the Composer's network and volume stores.
	containers, err := client.Containers(ctx, fmt.Sprintf("labels.%q", labels.ComposeProject))
	if err != nil {
		return err
	}
	records, err := projectContainers(ctx, containers)
	if err != nil {
		return err
	}
	return writeProjectList(cmd.OutOrStdout(), summarizeProjects(records, opts), opts)
}

func projectContainers(ctx context.Context, containers []containerd.Container) ([]projectContainer, error) {
	records := make([]projectContainer, 0, len(containers))
	for _, container := range containers {
		info, err := container.Info(ctx, containerd.WithoutRefreshedMetadata)
		if errdefs.IsNotFound(err) {
			// A container may disappear after the list call.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read compose labels for container %s: %w", container.ID(), err)
		}
		project := info.Labels[labels.ComposeProject]
		if project == "" || info.Labels[labels.ComposeService] == "" {
			continue
		}
		records = append(records, projectContainer{
			project:     project,
			state:       statusForFilter(ctx, container),
			configFiles: info.Labels[labels.ComposeProjectConfigFiles],
		})
	}
	return records, nil
}

func summarizeProjects(containers []projectContainer, opts projectListOptions) []projectSummary {
	type aggregate struct {
		states map[string]int
		files  map[string]struct{}
	}
	projects := make(map[string]*aggregate)
	for _, container := range containers {
		if container.project == "" || (!opts.all && container.state != "running") {
			continue
		}
		if !projectNameMatches(container.project, opts.filters) {
			continue
		}
		project := projects[container.project]
		if project == nil {
			project = &aggregate{states: make(map[string]int), files: make(map[string]struct{})}
			projects[container.project] = project
		}
		project.states[container.state]++
		for _, file := range strings.Split(container.configFiles, ",") {
			if file != "" {
				project.files[file] = struct{}{}
			}
		}
	}
	result := make([]projectSummary, 0, len(projects))
	for name, project := range projects {
		states := make([]string, 0, len(project.states))
		for state, count := range project.states {
			states = append(states, fmt.Sprintf("%s(%d)", state, count))
		}
		sort.Strings(states)
		files := make([]string, 0, len(project.files))
		for file := range project.files {
			files = append(files, file)
		}
		sort.Strings(files)
		configFiles := strings.Join(files, ",")
		if configFiles == "" {
			configFiles = "N/A"
		}
		result = append(result, projectSummary{Name: name, Status: strings.Join(states, ", "), ConfigFiles: configFiles})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func projectNameMatches(name string, filters []*regexp.Regexp) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if filter.MatchString(name) {
			return true
		}
	}
	return false
}

func writeProjectList(out io.Writer, projects []projectSummary, opts projectListOptions) error {
	if opts.quiet {
		for _, project := range projects {
			if _, err := fmt.Fprintln(out, project.Name); err != nil {
				return err
			}
		}
		return nil
	}
	if opts.format == "json" {
		return json.NewEncoder(out).Encode(projects)
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "NAME\tSTATUS\tCONFIG FILES"); err != nil {
		return err
	}
	for _, project := range projects {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", project.Name, project.Status, project.ConfigFiles); err != nil {
			return err
		}
	}
	return w.Flush()
}
