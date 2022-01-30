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
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
	httpapi "github.com/ipfs/go-ipfs-http-client"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ipath "github.com/ipfs/interface-go-ipfs-core/path"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// RegistryOptions represents options to configure the registry.
type RegistryOptions struct {

	// Times to retry query on IPFS. Zero or lower value means no retry.
	ReadRetryNum int

	// ReadTimeout is timeout duration of a read request to IPFS. Zero means no timeout.
	ReadTimeout time.Duration
}

func NewRegistry(ipfsClient iface.CoreAPI, options RegistryOptions) (http.Handler, error) {
	if _, ok := ipfsClient.(*httpapi.HttpApi); !ok {
		return nil, fmt.Errorf("IPFS client must be *HttpApi")
	}
	return &server{ipfsClient, options}, nil
}

// server is a read-only registry which converts OCI Distribution Spec's pull-related API to IPFS
// https://github.com/opencontainers/distribution-spec/blob/v1.0/spec.md#pull
type server struct {
	api    iface.CoreAPI
	config RegistryOptions
}

var manifestRegexp = regexp.MustCompile(`/v2/ipfs/([a-z0-9]+)/manifests/(.*)`)
var blobsRegexp = regexp.MustCompile(`/v2/ipfs/([a-z0-9]+)/blobs/(.*)`)

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cid, content, mediaType, size, err := s.serve(r)
	if err != nil {
		logrus.WithError(err).Warnf("failed to serve %q %q", r.Method, r.URL.Path)
		// TODO: support response body following OCI Distribution Spec's error response format spec:
		// https://github.com/opencontainers/distribution-spec/blob/v1.0/spec.md#error-codes
		http.Error(w, "", http.StatusNotFound)
		return
	}
	if content == nil {
		logrus.Debugf("returning without contents")
		w.WriteHeader(200)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	if r.Method == "GET" {
		http.ServeContent(w, r, "", time.Now(), content)
		logrus.WithField("CID", cid).Debugf("served file")
	}
	return
}

func (s *server) serve(r *http.Request) (string, io.ReadSeeker, string, int64, error) {
	if r.Method != "GET" && r.Method != "HEAD" {
		return "", nil, "", 0, fmt.Errorf("unsupported method")
	}

	if r.URL.Path == "/v2/" {
		logrus.Debugf("requested /v2/")
		return "", nil, "", 0, nil
	}

	if matches := manifestRegexp.FindStringSubmatch(r.URL.Path); len(matches) != 0 {
		cidStr, ref := matches[1], matches[2]
		if _, dgstErr := digest.Parse(ref); dgstErr == nil {
			resolvedCID, content, mediaType, size, err := s.serveContentByDigest(r.Context(), cidStr, ref)
			if !images.IsManifestType(mediaType) && !images.IsIndexType(mediaType) {
				return "", nil, "", 0, fmt.Errorf("cannot serve non-manifest from manifest API: %q", mediaType)
			}
			logrus.WithField("root CID", cidStr).WithField("digest", ref).WithField("resolved CID", resolvedCID).Debugf("resolved manifest by digest")
			return resolvedCID, content, mediaType, size, err
		}
		if ref != "latest" {
			return "", nil, "", 0, fmt.Errorf("tag of %q must be latest but got %q", cidStr, ref)
		}
		resolvedCID, content, mediaType, size, err := s.serveContentByCID(r.Context(), cidStr)
		if err != nil {
			return "", nil, "", 0, err
		}
		logrus.WithField("root CID", cidStr).WithField("resolved CID", resolvedCID).Debugf("resolved manifest by cid")
		return resolvedCID, content, mediaType, size, nil
	}

	if matches := blobsRegexp.FindStringSubmatch(r.URL.Path); len(matches) != 0 {
		rootCIDStr, dgstStr := matches[1], matches[2]
		resolvedCID, content, mediaType, size, err := s.serveContentByDigest(r.Context(), rootCIDStr, dgstStr)
		if err != nil {
			return "", nil, "", 0, err
		}
		logrus.WithField("root CID", rootCIDStr).WithField("digest", dgstStr).WithField("resolved CID", resolvedCID).Debugf("resolved blob by digest")
		return resolvedCID, content, mediaType, size, nil
	}

	return "", nil, "", 0, fmt.Errorf("unsupported path")
}

func (s *server) serveContentByCID(ctx context.Context, cidStr string) (resC string, r io.ReadSeeker, mediaType string, size int64, err error) {
	targetCID, err := cid.Decode(cidStr)
	if err != nil {
		return "", nil, "", 0, err
	}
	c, desc, err := s.resolveCIDOfRootBlob(ctx, targetCID)
	if err != nil {
		return "", nil, "", 0, err
	}
	rc, err := s.getReadSeeker(ctx, c)
	if err != nil {
		return "", nil, "", 0, err
	}
	return c.String(), rc, getMediaType(desc), desc.Size, nil
}

func (s *server) serveContentByDigest(ctx context.Context, rootCIDStr, digestStr string) (resC string, r io.ReadSeeker, mediaType string, size int64, err error) {
	rootCID, err := cid.Decode(rootCIDStr)
	if err != nil {
		return "", nil, "", 0, err
	}
	dgst, err := digest.Parse(digestStr)
	if err != nil {
		return "", nil, "", 0, err
	}
	_, rootDesc, err := s.resolveCIDOfRootBlob(ctx, rootCID)
	if err != nil {
		return "", nil, "", 0, err
	}
	targetCID, targetDesc, err := s.resolveCIDOfDigest(ctx, dgst, rootDesc)
	if err != nil {
		return "", nil, "", 0, err
	}
	rc, err := s.getReadSeeker(ctx, targetCID)
	if err != nil {
		return "", nil, "", 0, err
	}
	return targetCID.String(), rc, getMediaType(targetDesc), targetDesc.Size, nil
}

func (s *server) getReadSeeker(ctx context.Context, c cid.Cid) (io.ReadSeeker, error) {
	sr, err := s.getFile(ctx, c)
	if err != nil {
		return nil, err
	}
	return newBufReadSeeker(sr), nil
}

func (s *server) getFile(ctx context.Context, c cid.Cid) (*io.SectionReader, error) {
	target := ipath.IpfsPath(c) // only IPFS CID is supported
	n, err := s.api.Unixfs().Get(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("failed to get file %q: %v", c.String(), err)
	}
	f := files.ToFile(n)
	defer f.Close()
	size, err := f.Size()
	if err != nil {
		return nil, fmt.Errorf("failed to get size: %v", err)
	}
	ra := &retryReaderAt{
		ctx: ctx,
		readAtFunc: func(ctx context.Context, p []byte, off int64) (int, error) {
			resp, err := s.api.(*httpapi.HttpApi).Request("cat", target.String()).Option("offset", off).Option("length", len(p)).Send(ctx)
			if err != nil {
				return 0, err
			}
			if resp.Error != nil {
				return 0, resp.Error
			}
			defer resp.Output.Close()
			return io.ReadFull(resp.Output, p)
		},
		timeout: s.config.ReadTimeout,
		retry:   s.config.ReadRetryNum,
	}
	return io.NewSectionReader(ra, 0, size), nil
}

func (s *server) resolveCIDOfRootBlob(ctx context.Context, c cid.Cid) (cid.Cid, ocispec.Descriptor, error) {
	rc, err := s.getReadSeeker(ctx, c)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
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

func (s *server) resolveCIDOfDigest(ctx context.Context, dgst digest.Digest, desc ocispec.Descriptor) (cid.Cid, ocispec.Descriptor, error) {
	c, err := getIPFSCID(desc)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	if desc.Digest == dgst {
		return c, desc, nil // hit
	}
	if !images.IsManifestType(desc.MediaType) && !images.IsIndexType(desc.MediaType) {
		// This is not the target blob and have no child. Early return here and avoid querying this blob.
		return cid.Cid{}, ocispec.Descriptor{}, fmt.Errorf("blob doesn't match")
	}
	sr, err := s.getFile(ctx, c)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	descs, err := images.Children(ctx, &readerProvider{desc, sr}, desc)
	if err != nil {
		return cid.Cid{}, ocispec.Descriptor{}, err
	}
	var allErr error
	for _, desc := range descs {
		gotCID, gotDesc, err := s.resolveCIDOfDigest(ctx, dgst, desc)
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

type retryReaderAt struct {
	ctx        context.Context
	readAtFunc func(ctx context.Context, p []byte, off int64) (int, error)
	timeout    time.Duration
	retry      int
}

func (r *retryReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if r.retry < 0 {
		r.retry = 0
	}
	for i := 0; i <= r.retry; i++ {
		ctx := r.ctx
		if r.timeout != 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, r.timeout)
			defer cancel()
		}
		n, err := r.readAtFunc(ctx, p, off)
		if err == nil {
			return n, nil
		} else if !errors.Is(err, context.DeadlineExceeded) {
			return 0, err
		}
		// deadline exceeded. retry.
	}
	return 0, context.DeadlineExceeded
}

func newBufReadSeeker(rs io.ReadSeeker) io.ReadSeeker {
	rsc := &bufReadSeeker{
		rs: rs,
	}
	rsc.curR = bufio.NewReaderSize(rsc.rs, 512*1024)
	return rsc
}

type bufReadSeeker struct {
	rs   io.ReadSeeker
	curR *bufio.Reader
}

func (r *bufReadSeeker) Read(p []byte) (int, error) {
	return r.curR.Read(p)
}

func (r *bufReadSeeker) Seek(offset int64, whence int) (int64, error) {
	n, err := r.rs.Seek(offset, whence)
	if err != nil {
		return 0, err
	}
	r.curR.Reset(r.rs)
	return n, nil
}

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
