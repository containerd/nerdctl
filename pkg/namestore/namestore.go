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

package namestore

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/containerd/nerdctl/pkg/lockutil"
	"github.com/pkg/errors"
)

func New(dataStore, ns string) (NameStore, error) {
	dir := filepath.Join(dataStore, "names", ns)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	store := &nameStore{
		dir: dir,
	}
	return store, nil
}

type NameStore interface {
	Acquire(name, id string) error
	Release(name, id string) error
}

type nameStore struct {
	dir string
}

func (x *nameStore) Acquire(name, id string) error {
	if err := verifyName(name); err != nil {
		return err
	}
	if strings.TrimSpace(id) != id {
		return errors.Errorf("untrimmed ID %q", id)
	}
	fn := func() error {
		fileName := filepath.Join(x.dir, name)
		if b, err := ioutil.ReadFile(fileName); err == nil {
			return errors.Errorf("name %q is already used by ID %q", name, string(b))
		}
		return ioutil.WriteFile(fileName, []byte(id), 0600)
	}
	return lockutil.WithDirLock(x.dir, fn)
}

func (x *nameStore) Release(name, id string) error {
	if name == "" {
		return nil
	}
	if err := verifyName(name); err != nil {
		return err
	}
	if strings.TrimSpace(id) != id {
		return errors.Errorf("untrimmed ID %q", id)
	}
	fn := func() error {
		fileName := filepath.Join(x.dir, name)
		b, err := ioutil.ReadFile(fileName)
		if err != nil {
			if os.IsNotExist(err) {
				err = nil
			}
			return err
		}
		if s := strings.TrimSpace(string(b)); s != id {
			return errors.Errorf("name %q is used by ID %q, not by %q", name, s, id)
		}
		return os.RemoveAll(fileName)
	}
	return lockutil.WithDirLock(x.dir, fn)
}

func verifyName(name string) error {
	if name == "" {
		return errors.New("name is empty")
	}

	// TODO: find out docker-compatible regex
	if strings.ContainsAny(name, "/:\\") {
		return errors.Errorf("invalid name %q", name)
	}
	return nil
}
