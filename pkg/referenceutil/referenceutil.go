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
	"path"
	"strings"

	"github.com/distribution/reference"
	"github.com/ipfs/go-cid"
	"github.com/opencontainers/go-digest"
)

type Protocol string

const IPFSProtocol Protocol = "ipfs"
const IPNSProtocol Protocol = "ipns"
const shortIDLength = 5

type ImageReference struct {
	Protocol    Protocol
	Digest      digest.Digest
	Tag         string
	ExplicitTag string
	Path        string
	Domain      string

	nn reference.Reference
}

func (ir *ImageReference) Name() string {
	ret := ir.Domain
	if ret != "" {
		ret += "/"
	}
	ret += ir.Path
	return ret
}

func (ir *ImageReference) FamiliarName() string {
	if ir.Protocol != "" && ir.Domain == "" {
		return ir.Path
	}
	if ir.nn != nil {
		return reference.FamiliarName(ir.nn.(reference.Named))
	}
	return ""
}

func (ir *ImageReference) FamiliarMatch(pattern string) (bool, error) {
	return reference.FamiliarMatch(pattern, ir.nn)
}

func (ir *ImageReference) String() string {
	if ir.Protocol != "" && ir.Domain == "" {
		return ir.Path
	}
	if ir.Path == "" && ir.Digest != "" {
		return ir.Digest.String()
	}
	if ir.nn != nil {
		return ir.nn.String()
	}
	return ""
}

func (ir *ImageReference) SuggestContainerName(suffix string) string {
	name := "untitled"
	if ir.Protocol != "" && ir.Domain == "" {
		name = string(ir.Protocol) + "-" + ir.String()[:shortIDLength]
	} else if ir.Path != "" {
		name = path.Base(ir.Path)
	}
	return name + "-" + suffix[:5]
}

func Parse(rawRef string) (*ImageReference, error) {
	ir := &ImageReference{}

	if strings.HasPrefix(rawRef, "ipfs://") {
		ir.Protocol = IPFSProtocol
		rawRef = rawRef[7:]
	} else if strings.HasPrefix(rawRef, "ipns://") {
		ir.Protocol = IPNSProtocol
		rawRef = rawRef[7:]
	}
	if decodedCID, err := cid.Decode(rawRef); err == nil {
		ir.Protocol = IPFSProtocol
		rawRef = decodedCID.String()
		ir.Path = rawRef
		return ir, nil
	}

	if dgst, err := digest.Parse(rawRef); err == nil {
		ir.Digest = dgst
		return ir, nil
	} else if dgst, err := digest.Parse("sha256:" + rawRef); err == nil {
		ir.Digest = dgst
		return ir, nil
	}

	var err error
	ir.nn, err = reference.ParseNormalizedNamed(rawRef)
	if err != nil {
		return ir, err
	}
	if tg, ok := ir.nn.(reference.Tagged); ok {
		ir.ExplicitTag = tg.Tag()
	}
	if tg, ok := ir.nn.(reference.Named); ok {
		ir.nn = reference.TagNameOnly(tg)
		ir.Domain = reference.Domain(tg)
		ir.Path = reference.Path(tg)
	}
	if tg, ok := ir.nn.(reference.Tagged); ok {
		ir.Tag = tg.Tag()
	}
	if tg, ok := ir.nn.(reference.Digested); ok {
		ir.Digest = tg.Digest()
	}

	return ir, nil
}
