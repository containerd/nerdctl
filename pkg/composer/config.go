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
   Portions from https://github.com/docker/compose/blob/v2.2.2/pkg/compose/hash.go
   Copyright (C) Docker Compose authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/docker/compose/blob/v2.2.2/NOTICE
*/

package composer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/compose-spec/compose-go/types"
	"github.com/opencontainers/go-digest"
	"gopkg.in/yaml.v3"
)

type ConfigOptions struct {
	Services bool
	Volumes  bool
	Hash     string
}

func (c *Composer) Config(ctx context.Context, w io.Writer, co ConfigOptions) error {
	if co.Services {
		for _, service := range c.project.Services {
			fmt.Fprintln(w, service.Name)
		}
		return nil
	}
	if co.Volumes {
		for volume := range c.project.Volumes {
			fmt.Fprintln(w, volume)
		}
		return nil
	}
	if co.Hash != "" {
		var services []string
		if co.Hash != "*" {
			services = strings.Split(co.Hash, ",")
		}
		return c.project.WithServices(services, func(svc types.ServiceConfig) error {
			hash, err := ServiceHash(svc)
			if err != nil {
				return err
			}
			fmt.Fprintf(w, "%s %s\n", svc.Name, hash)
			return nil
		})
	}
	projectYAML, err := yaml.Marshal(c.project)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "%s", projectYAML)
	return nil
}

// ServiceHash is from https://github.com/docker/compose/blob/v2.2.2/pkg/compose/hash.go#L28-L38
func ServiceHash(o types.ServiceConfig) (string, error) {
	// remove the Build config when generating the service hash
	o.Build = nil
	o.PullPolicy = ""
	o.Scale = 1
	bytes, err := json.Marshal(o)
	if err != nil {
		return "", err
	}
	return digest.SHA256.FromBytes(bytes).Encoded(), nil
}
