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

package referenceutil

import (
	"fmt"
	"path"
	"strings"

	refdocker "github.com/containerd/containerd/reference/docker"
	"github.com/ipfs/go-cid"
)

// Reference is a reference to an image.
type Reference interface {

	// String returns the full reference which can be understood by containerd.
	String() string
}

// ParseAnyReference parses the passed reference as IPFS, CID, or a classic reference.
// Unlike ParseAny, it is not limited to the DockerRef limitations (being either tagged or digested)
// and should be used instead.
func ParseAnyReference(rawRef string) (Reference, error) {
	if scheme, ref, err := ParseIPFSRefWithScheme(rawRef); err == nil {
		return Reference(stringRef{scheme: scheme, s: ref}), nil
	}
	if c, err := cid.Decode(rawRef); err == nil {
		return c, nil
	}
	return refdocker.ParseAnyReference(rawRef)
}

// ParseAny parses the passed reference with allowing it to be non-docker reference.
// If the ref has IPFS scheme or can be parsed as CID, it's parsed as an IPFS reference.
// Otherwise it's parsed as a docker reference.
func ParseAny(rawRef string) (Reference, error) {
	if scheme, ref, err := ParseIPFSRefWithScheme(rawRef); err == nil {
		return stringRef{scheme: scheme, s: ref}, nil
	}
	if c, err := cid.Decode(rawRef); err == nil {
		return c, nil
	}
	return ParseDockerRef(rawRef)
}

// ParseDockerRef parses the passed reference with assuming it's a docker reference.
func ParseDockerRef(rawRef string) (refdocker.Named, error) {
	return refdocker.ParseDockerRef(rawRef)
}

// ParseIPFSRefWithScheme parses the passed reference with assuming it's an IPFS reference with scheme prefix.
func ParseIPFSRefWithScheme(name string) (scheme, ref string, err error) {
	if strings.HasPrefix(name, "ipfs://") || strings.HasPrefix(name, "ipns://") {
		return name[:4], name[7:], nil
	}
	return "", "", fmt.Errorf("reference is not an IPFS reference")
}

type stringRef struct {
	scheme string
	s      string
}

func (s stringRef) String() string {
	return s.s
}

// SuggestContainerName generates a container name from name.
// The result MUST NOT be parsed.
func SuggestContainerName(rawRef, containerID string) string {
	const shortIDLength = 5
	if len(containerID) < shortIDLength {
		panic(fmt.Errorf("got too short (< %d) container ID: %q", shortIDLength, containerID))
	}
	name := "untitled-" + containerID[:shortIDLength]
	if rawRef != "" {
		r, err := ParseAny(rawRef)
		if err == nil {
			switch rr := r.(type) {
			case refdocker.Named:
				if rrName := rr.Name(); rrName != "" {
					imageNameBased := path.Base(rrName)
					if imageNameBased != "" {
						name = imageNameBased + "-" + containerID[:shortIDLength]
					}
				}
			case cid.Cid:
				name = "ipfs" + "-" + rr.String()[:shortIDLength] + "-" + containerID[:shortIDLength]
			case stringRef:
				name = rr.scheme + "-" + rr.s[:shortIDLength] + "-" + containerID[:shortIDLength]
			}
		}
	}
	return name
}
