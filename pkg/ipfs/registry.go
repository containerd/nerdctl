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

package ipfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/images"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ipath "github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

func NewRegistry(ipfsClient iface.CoreAPI) http.Handler {
	return &server{ipfsClient}
}

// server is a read-only registry which converts OCI Distribution Spec's pull-related API to IPFS
// https://github.com/opencontainers/distribution-spec/blob/v1.0/spec.md#pull
type server struct {
	api iface.CoreAPI
}

var manifestRegexp = regexp.MustCompile(`/v2/ipfs/([a-z0-9]+)/manifests/(.*)`)
var blobsRegexp = regexp.MustCompile(`/v2/ipfs/([a-z0-9]+)/blobs/(.*)`)

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	content, mediaType, size, err := s.serve(r)
	if err != nil {
		logrus.WithError(err).Warnf("failed to serve %q %q", r.Method, r.URL.Path)
		// TODO: support response body following OCI Distribution Spec's error response format spec:
		// https://github.com/opencontainers/distribution-spec/blob/v1.0/spec.md#error-codes
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if content == nil {
		w.WriteHeader(200)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	if r.Method == "GET" {
		http.ServeContent(w, r, "", time.Now(), content)
	}
	if err := content.Close(); err != nil {
		logrus.WithError(err).Warnf("failed to close file")
	}
	return
}

func (s *server) serve(r *http.Request) (io.ReadSeekCloser, string, int64, error) {
	if r.Method != "GET" && r.Method != "HEAD" {
		return nil, "", 0, fmt.Errorf("unsupported method")
	}

	if r.URL.Path == "/v2/" {
		return nil, "", 0, nil
	}

	if matches := manifestRegexp.FindStringSubmatch(r.URL.Path); len(matches) != 0 {
		cidStr, ref := matches[1], matches[2]
		if _, dgstErr := digest.Parse(ref); dgstErr == nil {
			content, mediaType, size, err := s.serveContentByDigest(cidStr, ref)
			if !images.IsManifestType(mediaType) && !images.IsIndexType(mediaType) {
				return nil, "", 0, fmt.Errorf("cannot serve non-manifest from manifets API: %q", mediaType)
			}
			return content, mediaType, size, err
		}
		if ref != "latest" {
			return nil, "", 0, fmt.Errorf("tag of %q must be latest but got %q", cidStr, ref)
		}
		return s.serveContentByCID(cidStr)
	}

	if matches := blobsRegexp.FindStringSubmatch(r.URL.Path); len(matches) != 0 {
		rootCIDStr, dgstStr := matches[1], matches[2]
		return s.serveContentByDigest(rootCIDStr, dgstStr)
	}

	return nil, "", 0, fmt.Errorf("unsupported path")
}

func (s *server) serveContentByCID(cidStr string) (r io.ReadSeekCloser, mediaType string, size int64, err error) {
	targetCID, err := cid.Decode(cidStr)
	if err != nil {
		return nil, "", 0, err
	}
	c, desc, err := s.resolveCIDOfRootBlob(targetCID)
	if err != nil {
		return nil, "", 0, err
	}
	rc, err := s.getReadSeekCloser(c)
	if err != nil {
		return nil, "", 0, err
	}
	return rc, getMediaType(desc), desc.Size, nil
}

func (s *server) serveContentByDigest(rootCIDStr, digestStr string) (r io.ReadSeekCloser, mediaType string, size int64, err error) {
	rootCID, err := cid.Decode(rootCIDStr)
	if err != nil {
		return nil, "", 0, err
	}
	dgst, err := digest.Parse(digestStr)
	if err != nil {
		return nil, "", 0, err
	}
	_, rootDesc, err := s.resolveCIDOfRootBlob(rootCID)
	if err != nil {
		return nil, "", 0, err
	}
	targetCID, targetDesc, err := s.resolveCIDOfDigest(dgst, rootDesc)
	if err != nil {
		return nil, "", 0, err
	}
	rc, err := s.getReadSeekCloser(targetCID)
	if err != nil {
		return nil, "", 0, err
	}
	return rc, getMediaType(targetDesc), targetDesc.Size, nil
}

func (s *server) getReadSeekCloser(c cid.Cid) (io.ReadSeekCloser, error) {
	r, closeFunc, err := s.getFile(c)
	if err != nil {
		return nil, err
	}
	return &readSeekCloser{r, closeFunc}, nil
}

func (s *server) getFile(c cid.Cid) (*io.SectionReader, func() error, error) {
	n, err := s.api.Unixfs().Get(context.Background(), ipath.IpfsPath(c)) // only IPFS CID is supported
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file %q: %v", c.String(), err)
	}
	f := files.ToFile(n)
	ra, ok := f.(interface {
		io.ReaderAt
	})
	if !ok {
		return nil, nil, fmt.Errorf("ReaderAt is not implemented")
	}
	size, err := f.Size()
	if err != nil {
		return nil, nil, err
	}
	return io.NewSectionReader(ra, 0, size), f.Close, nil
}

func (s *server) resolveCIDOfRootBlob(c cid.Cid) (cid.Cid, ocispec.Descriptor, error) {
	rc, err := s.getReadSeekCloser(c)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	defer rc.Close()
	var desc ocispec.Descriptor
	if err := json.NewDecoder(rc).Decode(&desc); err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	c, err = getIPFSCID(desc)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	return c, desc, nil
}

func (s *server) resolveCIDOfDigest(dgst digest.Digest, desc ocispec.Descriptor) (cid.Cid, ocispec.Descriptor, error) {
	c, err := getIPFSCID(desc)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	if desc.Digest == dgst {
		return c, desc, nil // hit
	}
	sr, closeFunc, err := s.getFile(c)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	descs, err := images.Children(context.Background(), &readerProvider{desc, sr}, desc)
	if err != nil {
		closeFunc()
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	if err := closeFunc(); err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	var allErr error
	for _, desc := range descs {
		gotCID, gotDesc, err := s.resolveCIDOfDigest(dgst, desc)
		if err != nil {
			allErr = multierror.Append(allErr, err)
			continue
		}
		return gotCID, gotDesc, nil
	}
	if allErr == nil {
		return cid.Cid{}, ocispec.Descriptor{}, fmt.Errorf("not found")
	}
	return cid.Cid{}, ocispec.Descriptor{}, allErr
}

func getIPFSCID(desc ocispec.Descriptor) (cid.Cid, error) {
	for _, u := range desc.URLs {
		if strings.HasPrefix(u, "ipfs://") {
			// support only content addressable URL (ipfs://<CID>)
			return cid.Decode(u[7:])
		}
	}
	return cid.Cid{}, fmt.Errorf("no CID is recorded in %s", desc.Digest)
}

func getMediaType(desc ocispec.Descriptor) string {
	if images.IsManifestType(desc.MediaType) || images.IsIndexType(desc.MediaType) || images.IsConfigType(desc.MediaType) {
		return desc.MediaType
	}
	return "application/octet-stream"
}

type readSeekCloser struct {
	io.ReadSeeker
	closeFunc func() error
}

func (r *readSeekCloser) Close() error { return r.closeFunc() }

type readerProvider struct {
	desc ocispec.Descriptor
	r    *io.SectionReader
}

func (p *readerProvider) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	if desc.Digest != p.desc.Digest || desc.Size != p.desc.Size {
		return nil, fmt.Errorf("unexpected content")
	}
	return &contentReaderAt{p.r}, nil
}

type contentReaderAt struct {
	*io.SectionReader
}

func (r *contentReaderAt) Close() error { return nil }
