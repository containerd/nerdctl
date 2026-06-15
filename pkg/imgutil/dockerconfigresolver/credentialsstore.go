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

package dockerconfigresolver

import (
	"errors"
	"fmt"
	"strings"

	"github.com/docker/cli/cli/config"
	"github.com/docker/cli/cli/config/configfile"
	"github.com/docker/cli/cli/config/types"
)

type Credentials = types.AuthConfig

// NewCredentialsStore returns a CredentialsStore from a directory
// If path is left empty, the default docker `~/.docker/config.json` will be used
// In case the docker call fails, we wrap the error with ErrUnableToInstantiate
func NewCredentialsStore(path string) (*CredentialsStore, error) {
	dockerConfigFile, err := config.Load(path)
	if err != nil {
		return nil, errors.Join(ErrUnableToInstantiate, err)
	}

	return &CredentialsStore{
		dockerConfigFile: dockerConfigFile,
	}, nil
}

// CredentialsStore is an abstraction in front of docker config API manipulation
// exposing just the limited functions we need and hiding away url normalization / identifiers magic, and handling of
// backward compatibility
type CredentialsStore struct {
	dockerConfigFile *configfile.ConfigFile
}

// Erase will remove any and all stored credentials for that registry namespace (including all legacy variants)
// If we do not find at least ONE variant matching the namespace, this will error with ErrUnableToErase
func (cs *CredentialsStore) Erase(registryURL *RegistryURL) (map[string]error, error) {
	// Get all associated identifiers for that registry including legacy ones and variants
	logoutList := registryURL.AllIdentifiers()

	// Iterate through and delete them one by one
	errs := make(map[string]error)
	for _, serverAddress := range logoutList {
		if err := cs.dockerConfigFile.GetCredentialsStore(serverAddress).Erase(serverAddress); err != nil {
			errs[serverAddress] = err
		}
	}

	// If we succeeded removing at least one, it is a success.
	// The only error condition is if we failed removing anything - meaning there was no such credential information
	// in whatever format - or the store is broken.
	if len(errs) == len(logoutList) {
		return errs, ErrUnableToErase
	}

	return nil, nil
}

// Store will save credentials for a given registry
// On error, ErrUnableToStore
func (cs *CredentialsStore) Store(registryURL *RegistryURL, credentials *Credentials) error {
	// We just overwrite the server property here with the host
	// Whether it was one of the variants, or was not set at all (see for example Amazon ECR, https://github.com/containerd/nerdctl/issues/733
	// - which is likely a bug in docker) it doesn't matter.
	// This is the credentials that were returned for that host, by the docker credentials store.
	if registryURL.Namespace != nil {
		credentials.ServerAddress = fmt.Sprintf("%s%s?%s", registryURL.Host, registryURL.Path, registryURL.RawQuery)
	} else {
		credentials.ServerAddress = registryURL.CanonicalIdentifier()
	}

	// XXX future namespaced url likely require special handling here
	if err := cs.dockerConfigFile.GetCredentialsStore(registryURL.CanonicalIdentifier()).Store(*(credentials)); err != nil {
		return errors.Join(ErrUnableToStore, err)
	}

	return nil
}

// ShellCompletion will return candidate strings for nerdctl logout
func (cs *CredentialsStore) ShellCompletion() []string {
	candidates := []string{}
	for key := range cs.dockerConfigFile.AuthConfigs {
		candidates = append(candidates, key)
	}

	return candidates
}

// FileStorageLocation will return the file where credentials are stored for a given registry, or the empty string
// if it is stored / to be stored in a different place (like an OS keychain, with docker credential helpers)
func (cs *CredentialsStore) FileStorageLocation(registryURL *RegistryURL) string {
	if store, isFile := (cs.dockerConfigFile.GetCredentialsStore(registryURL.CanonicalIdentifier())).(isFileStore); isFile {
		return store.GetFilename()
	}

	return ""
}

// Retrieve gets existing credentials from the store for a certain registry.
// If none are found, an empty Credentials struct is returned.
// If we hard-fail reading from the store, indicative of a broken system, we wrap the error with ErrUnableToRetrieve
func (cs *CredentialsStore) Retrieve(registryURL *RegistryURL, checkCredStore bool) (*Credentials, error) {
	var err error
	returnedCredentials := &Credentials{}

	// As long as we depend on .ServerAddress, make sure it is populated correctly
	// It does not matter what was stored - the docker cli clearly has issues with this
	// What matters is that the credentials retrieved from the docker credentials store are *for that registryURL*
	// and that is what ServerAddress should point to
	defer func() {
		if registryURL.Namespace != nil {
			returnedCredentials.ServerAddress = fmt.Sprintf("%s%s?%s", registryURL.Host, registryURL.Path, registryURL.RawQuery)
		} else {
			returnedCredentials.ServerAddress = registryURL.Host
		}
	}()

	if !checkCredStore {
		return returnedCredentials, nil
	}

	// Get the legacy variants (w/o scheme or port), and iterate over until we find one with credentials
	variants := registryURL.AllIdentifiers()

	for _, identifier := range variants {
		var credentials types.AuthConfig
		// Note that Get does not raise an error on ENOENT
		credentials, err = cs.dockerConfigFile.GetCredentialsStore(identifier).Get(identifier)
		if err != nil {
			continue
		}
		returnedCredentials = &credentials
		// Clean-up the username
		returnedCredentials.Username = strings.TrimSpace(returnedCredentials.Username)
		// Stop here if we found credentials with this variant
		if returnedCredentials.IdentityToken != "" ||
			returnedCredentials.Username != "" ||
			returnedCredentials.Password != "" ||
			returnedCredentials.RegistryToken != "" {
			break
		}
	}

	// (Last non nil) credential store error gets wrapped into ErrUnableToRetrieve
	if err != nil {
		err = errors.Join(ErrUnableToRetrieve, err)
	}

	return returnedCredentials, err
}

// isFileStore is an internal mock interface purely meant to help identify that the docker credential backend is a filesystem one
type isFileStore interface {
	IsFileStore() bool
	GetFilename() string
}
