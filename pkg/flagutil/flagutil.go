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

package flagutil

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/strutil"
)

// ReplaceOrAppendEnvValues returns the defaults with the overrides either
// replaced by env key or appended to the list
// FYI: https://github.com/containerd/containerd/blob/698622b89a053294593b9b5a363efff7715e9394/oci/spec_opts.go#L186-L222
// defaults should have valid `k=v` strings.
// overrides may have the following formats: `k=v` (override k), `k=` (emptify k), `k` (remove k).
func ReplaceOrAppendEnvValues(defaults, overrides []string) []string {
	cache := make(map[string]int, len(defaults))
	results := make([]string, 0, len(defaults))
	for i, e := range defaults {
		k, _, _ := strings.Cut(e, "=")
		results = append(results, e)
		cache[k] = i
	}

	for _, value := range overrides {
		// Values w/o = means they want this env to be removed/unset.
		k, _, ok := strings.Cut(value, "=")
		if !ok {
			if i, exists := cache[k]; exists {
				results[i] = "" // Used to indicate it should be removed
			}
			continue
		}

		// Just do a normal set/update
		if i, exists := cache[k]; exists {
			results[i] = value
		} else {
			results = append(results, value)
		}
	}

	// Now remove all entries that we want to "unset"
	for i := 0; i < len(results); i++ {
		if results[i] == "" {
			results = append(results[:i], results[i+1:]...)
			i--
		}
	}

	return results
}

func parseEnvVars(paths []string) ([]string, error) {
	vars := make([]string, 0)
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open env file %s: %w", path, err)
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			// skip comment lines and empty line
			if len(line) == 0 || strings.HasPrefix(line, "#") {
				continue
			}
			vars = append(vars, line)
		}
		if err = sc.Err(); err != nil {
			return nil, err
		}
	}
	return vars, nil
}

func withOSEnv(envs []string) ([]string, error) {
	newEnvs := make([]string, len(envs))

	// from https://github.com/docker/cli/blob/v22.06.0-beta.0/opts/env.go#L18
	getEnv := func(val string) (string, error) {
		arr := strings.SplitN(val, "=", 2)
		if arr[0] == "" {
			return "", errors.New("invalid environment variable: " + val)
		}
		if len(arr) > 1 {
			return val, nil
		}
		if envVal, ok := os.LookupEnv(arr[0]); ok {
			return arr[0] + "=" + envVal, nil
		}
		return val, nil
	}
	for i := range envs {
		env, err := getEnv(envs[i])
		if err != nil {
			return nil, err
		}
		newEnvs[i] = env
	}

	return newEnvs, nil
}

// MergeEnvFileAndOSEnv combines environment variables from `--env-file` and `--env`.
// Pass an empty slice if any arg is not used.
func MergeEnvFileAndOSEnv(envFile []string, env []string) ([]string, error) {
	var envs []string
	var err error

	if envFiles := strutil.DedupeStrSlice(envFile); len(envFiles) > 0 {
		envs, err = parseEnvVars(envFiles)
		if err != nil {
			return nil, err
		}
	}

	if env := strutil.DedupeStrSlice(env); len(env) > 0 {
		envs = append(envs, env...)
	}

	if envs, err = withOSEnv(envs); err != nil {
		return nil, err
	}

	return envs, nil
}
