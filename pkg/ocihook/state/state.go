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

package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/lockutil"
)

// This is meant to store stateful informations about containers that we receive from ocihooks
// We are storing them inside the container statedir
// Note that you MUST use WithLock to perform any operation (like Read or Write).
// Typically:
// lf.WithLock(func ()error {
//   lf.Load()
//   // Modify something on the object
//   lf.StartedAt = ...
//   lf.Save()
// })

const (
	lifecycleFile = "lifecycle.json"
)

func NewLifecycleState(stateDir string) *LifecycleState {
	return &LifecycleState{
		stateDir: stateDir,
	}
}

type LifecycleState struct {
	stateDir    string
	StartedAt   time.Time `json:"started_at"`
	CreateError bool      `json:"create_error"`
}

func (lf *LifecycleState) WithLock(fun func() error) error {
	err := lockutil.WithDirLock(lf.stateDir, fun)
	if err != nil {
		return fmt.Errorf("failed to lock state dir: %w", err)
	}

	return nil
}

func (lf *LifecycleState) Load() error {
	data, err := os.ReadFile(filepath.Join(lf.stateDir, lifecycleFile))
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("unable to read lifecycle file: %w", err)
		}
	} else {
		err = json.Unmarshal(data, lf)
		if err != nil {
			// Logging an error, as Load errors are generally ignored downstream
			log.L.Error("unable to unmarshall lifecycle data")
			return fmt.Errorf("unable to unmarshall lifecycle data: %w", err)
		}
	}
	return nil
}

func (lf *LifecycleState) Save() error {
	// Write atomically (write, then move) to avoid incomplete writes from happening
	data, err := json.Marshal(lf)
	if err != nil {
		return fmt.Errorf("unable to marshall lifecycle data: %w", err)
	}
	err = os.WriteFile(filepath.Join(lf.stateDir, "."+lifecycleFile), data, 0600)
	if err != nil {
		return fmt.Errorf("unable to write lifecycle file: %w", err)
	}
	err = os.Rename(filepath.Join(lf.stateDir, "."+lifecycleFile), filepath.Join(lf.stateDir, lifecycleFile))
	if err != nil {
		return fmt.Errorf("unable to write lifecycle file: %w", err)
	}
	return nil
}
