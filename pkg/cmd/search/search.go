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

package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/tabwriter"

	dockerconfig "github.com/containerd/containerd/v2/core/remotes/docker/config"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/formatter"
	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
)

type SearchResult struct {
	Description string `json:"description"`
	IsOfficial  bool   `json:"is_official"`
	Name        string `json:"name"`
	StarCount   int    `json:"star_count"`
}

func Search(ctx context.Context, term string, options types.SearchOptions) error {
	// Validate filters before making HTTP request
	filterMap, err := validateAndParseFilters(options.Filters)
	if err != nil {
		return err
	}

	registryHost, searchTerm := splitReposSearchTerm(term)

	parsedRef, err := referenceutil.Parse(registryHost)
	if err != nil {
		log.G(ctx).WithError(err).Debugf("failed to parse registry host %q, using as-is", registryHost)
	} else {
		registryHost = parsedRef.Domain
	}

	var dOpts []dockerconfigresolver.Opt

	if options.GOptions.InsecureRegistry {
		log.G(ctx).Warnf("skipping verifying HTTPS certs for %q", registryHost)
		dOpts = append(dOpts, dockerconfigresolver.WithSkipVerifyCerts(true))
	}

	dOpts = append(dOpts, dockerconfigresolver.WithHostsDirs(options.GOptions.HostsDir))

	hostOpts, err := dockerconfigresolver.NewHostOptions(ctx, registryHost, dOpts...)
	if err != nil {
		return fmt.Errorf("failed to create host options: %w", err)
	}

	username, password, err := hostOpts.Credentials(registryHost)
	if err != nil {
		log.G(ctx).WithError(err).Debug("no credentials found, searching anonymously")
	}

	scheme := "https"
	if hostOpts.DefaultScheme != "" {
		scheme = hostOpts.DefaultScheme
	}

	searchURL := buildSearchURL(registryHost, searchTerm, scheme)

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return err
	}

	if username != "" && password != "" {
		req.SetBasicAuth(username, password)
	}

	client := createHTTPClient(hostOpts)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp struct {
		Results []SearchResult `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return fmt.Errorf("failed to decode search response: %w", err)
	}

	filteredResults := applyFilters(searchResp.Results, filterMap, options.Limit)

	return printSearchResults(options.Stdout, filteredResults, options)
}

func splitReposSearchTerm(reposName string) (registryHost string, searchTerm string) {
	nameParts := strings.SplitN(reposName, "/", 2)
	if len(nameParts) == 1 ||
		(!strings.Contains(nameParts[0], ".") &&
			!strings.Contains(nameParts[0], ":") &&
			nameParts[0] != "localhost") {
		// No registry specified, use docker.io
		// For "library/alpine", the search term should be "alpine"
		// For "alpine", the search term should be "alpine"
		if len(nameParts) == 2 && nameParts[0] == "library" {
			return "docker.io", nameParts[1]
		}
		return "docker.io", reposName
	}
	return nameParts[0], nameParts[1]
}

func buildSearchURL(registryHost, term, scheme string) string {
	host := registryHost
	if host == "docker.io" {
		host = "index.docker.io"
	}

	u := url.URL{
		Scheme: scheme,
		Host:   host,
		Path:   "/v1/search",
	}
	q := u.Query()
	q.Set("q", term)
	u.RawQuery = q.Encode()

	return u.String()
}

func createHTTPClient(hostOpts *dockerconfig.HostOptions) *http.Client {
	if hostOpts != nil && hostOpts.DefaultTLS != nil {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: hostOpts.DefaultTLS,
			},
		}
	}
	return http.DefaultClient
}

func validateFilterValue(key, value string) error {
	switch key {
	case "stars":
		if _, err := strconv.Atoi(value); err != nil {
			return fmt.Errorf("invalid filter 'stars=%s'", value)
		}
	case "is-official":
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("invalid filter 'is-official=%s'", value)
		}
	default:
		return fmt.Errorf("invalid filter '%s'", key)
	}
	return nil
}

// validateAndParseFilters validates and parses filters before making HTTP request
func validateAndParseFilters(filters []string) (map[string]string, error) {
	filterMap := make(map[string]string)
	for _, f := range filters {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad format of filter (expected name=value)")
		}
		key := parts[0]
		value := parts[1]
		if err := validateFilterValue(key, value); err != nil {
			return nil, err
		}
		filterMap[key] = value
	}
	return filterMap, nil
}

func applyFilters(results []SearchResult, filterMap map[string]string, limit int) []SearchResult {
	filtered := make([]SearchResult, 0, len(results))

	for _, r := range results {
		if val, ok := filterMap["is-official"]; ok {
			b, _ := strconv.ParseBool(val)
			if b != r.IsOfficial {
				continue
			}
		}

		if val, ok := filterMap["stars"]; ok {
			stars, _ := strconv.Atoi(val)
			if r.StarCount < stars {
				continue
			}
		}

		filtered = append(filtered, r)
	}

	// Apply limit after filtering, but maintain original order from API
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return filtered
}

func truncateDescription(desc string, noTrunc bool) string {
	if !noTrunc && len(desc) > 45 {
		return formatter.Ellipsis(desc, 45)
	}
	return desc
}

func printSearchResults(stdout io.Writer, results []SearchResult, options types.SearchOptions) error {
	for i := range results {
		results[i].Description = truncateDescription(results[i].Description, options.NoTrunc)
	}

	if options.Format != "" {
		tmpl, err := formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
		for _, r := range results {
			if err := tmpl.Execute(stdout, r); err != nil {
				return err
			}
			fmt.Fprintln(stdout)
		}
		return nil
	}

	w := tabwriter.NewWriter(stdout, 20, 1, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tSTARS\tOFFICIAL")

	for _, r := range results {
		desc := strings.ReplaceAll(r.Description, "\n", " ")
		desc = strings.ReplaceAll(desc, "\t", " ")

		official := ""
		if r.IsOfficial {
			official = "[OK]"
		}
		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", r.Name, desc, r.StarCount, official)
	}
	return w.Flush()
}
