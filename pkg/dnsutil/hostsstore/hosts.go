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

/*
  Portions from https://github.com/jaytaylor/go-hostsfile/blob/59e7508e09b9e08c57183ae15eabf1b757328ebf/hosts.go
  Copyright (c) 2016 Jay Taylor [@jtaylor]

  Permission is hereby granted, free of charge, to any person obtaining a copy
  of this software and associated documentation files (the "Software"), to deal
  in the Software without restriction, including without limitation the rights
  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
  copies of the Software, and to permit persons to whom the Software is
  furnished to do so, subject to the following conditions:

  The above copyright notice and this permission notice shall be included in all
  copies or substantial portions of the Software.
*/

package hostsstore

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	MarkerBegin = "<nerdctl>"
	MarkerEnd   = "</nerdctl>"
)

// parseHostsButSkipMarkedRegion parses hosts file content but skips the <nerdctl> </nerdctl> region
// mimics  https://github.com/jaytaylor/go-hostsfile/blob/59e7508e09b9e08c57183ae15eabf1b757328ebf/hosts.go#L18
// mimics  https://github.com/norouter/norouter/blob/v0.6.2/pkg/agent/etchosts/etchosts.go#L128-L152
func parseHostsButSkipMarkedRegion(w io.Writer, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	skip := false
LINE:
	for scanner.Scan() {
		line := scanner.Text()
		line = strings.Replace(strings.Trim(line, " \t"), "\t", " ", -1)
		sawMarkerEnd := false
		if strings.HasPrefix(line, "#") {
			com := strings.TrimSpace(line[1:])
			switch com {
			case MarkerBegin:
				skip = true
			case MarkerEnd:
				sawMarkerEnd = true
			}
		}
		if !skip {
			if len(line) == 0 || line[0] == ';' || line[0] == '#' {
				continue
			}
			pieces := strings.SplitN(line, " ", 2)
			if len(pieces) > 1 && len(pieces[0]) > 0 {
				if pieces[0] == "127.0.0.1" || pieces[0] == "::1" {
					continue LINE
				}
				if names := strings.Fields(pieces[1]); len(names) > 0 {
					for _, name := range names {
						if strings.HasPrefix(name, "#") {
							continue LINE
						}
					}
					fmt.Fprintln(w, line)
				}
			}
		}
		if sawMarkerEnd {
			skip = false
		}
	}
	return scanner.Err()
}
