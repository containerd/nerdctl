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

package manifeststore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/containerd/nerdctl/v2/pkg/manifesttypes"
	"github.com/containerd/nerdctl/v2/pkg/referenceutil"
	"github.com/containerd/nerdctl/v2/pkg/store"
)

type Store interface {
	Get(listRef *referenceutil.ImageReference, manifestRef *referenceutil.ImageReference) (*manifesttypes.DockerManifestEntry, error)
	// GetList returns all the local manifests for a index or manifest list
	GetList(listRef *referenceutil.ImageReference) ([]*manifesttypes.DockerManifestEntry, error)
	// Save saves a manifest as part of a index or local manifest list
	Save(listRef, manifestRef *referenceutil.ImageReference, manifest *manifesttypes.DockerManifestEntry) error
}

type manifestStore struct {
	store store.Store
}

func NewStore(dataRoot string) (Store, error) {
	manifestRoot := filepath.Join(dataRoot, "manifests")
	st, err := store.New(manifestRoot, 0o755, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest store: %w", err)
	}
	return &manifestStore{store: st}, nil
}

func (s *manifestStore) Get(listRef *referenceutil.ImageReference, manifestRef *referenceutil.ImageReference) (*manifesttypes.DockerManifestEntry, error) {
	var manifest *manifesttypes.DockerManifestEntry
	err := s.store.WithLock(func() error {
		listPath := makeFilesafeName(listRef.String())
		manifestPath := makeFilesafeName(manifestRef.String())

		var err error
		manifest, err = s.getManifestFromPath(listPath, manifestPath)
		return err
	})
	return manifest, err
}

func (s *manifestStore) GetList(listRef *referenceutil.ImageReference) ([]*manifesttypes.DockerManifestEntry, error) {
	listPath := makeFilesafeName(listRef.String())

	if err := s.store.Lock(); err != nil {
		return nil, err
	}
	defer s.store.Release()

	manifestPaths, err := s.store.List(listPath)
	if err != nil {
		return nil, err
	}

	var manifests []*manifesttypes.DockerManifestEntry
	for _, manifestPath := range manifestPaths {
		manifest, err := s.getManifestFromPath(listPath, manifestPath)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}

	return manifests, nil
}

func (s *manifestStore) Save(listRef, manifestRef *referenceutil.ImageReference, manifest *manifesttypes.DockerManifestEntry) error {
	return s.store.WithLock(func() error {
		listPath := makeFilesafeName(listRef.String())
		if err := s.store.GroupEnsure(listPath); err != nil {
			return err
		}

		manifestPath := makeFilesafeName(manifestRef.String())
		data, err := json.Marshal(manifest)
		if err != nil {
			return err
		}

		return s.store.Set(data, listPath, manifestPath)
	})
}

func (s *manifestStore) getManifestFromPath(listPath, manifestPath string) (*manifesttypes.DockerManifestEntry, error) {
	data, err := s.store.Get(listPath, manifestPath)
	if err != nil {
		return nil, err
	}

	var manifest manifesttypes.DockerManifestEntry
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &manifest, nil
}

func makeFilesafeName(ref string) string {
	fileName := strings.ReplaceAll(ref, ":", "-")
	return strings.ReplaceAll(fileName, "/", "_")
}
