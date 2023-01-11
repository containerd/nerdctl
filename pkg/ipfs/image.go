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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/images/converter"
	"github.com/containerd/containerd/remotes"
	"github.com/containerd/nerdctl/pkg/idutil/imagewalker"
	"github.com/containerd/nerdctl/pkg/imgutil"
	"github.com/containerd/nerdctl/pkg/platformutil"
	"github.com/containerd/nerdctl/pkg/referenceutil"
	"github.com/containerd/stargz-snapshotter/ipfs"
	"github.com/docker/docker/errdefs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/sirupsen/logrus"
)

// EnsureImage pull the specified image from IPFS.
func EnsureImage(ctx context.Context, client *containerd.Client, stdout, stderr io.Writer, snapshotter string, scheme string, ref string, mode imgutil.PullMode, ocispecPlatforms []ocispec.Platform, unpack *bool, quiet bool, ipfsPath *string) (*imgutil.EnsuredImage, error) {
	switch mode {
	case "always", "missing", "never":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected pull mode: %q", mode)
	}
	switch scheme {
	case "ipfs", "ipns":
		// NOP
	default:
		return nil, fmt.Errorf("unexpected scheme: %q", scheme)
	}

	if mode != "always" && len(ocispecPlatforms) == 1 {
		res, err := imgutil.GetExistingImage(ctx, client, snapshotter, ref, ocispecPlatforms[0])
		if err == nil {
			return res, nil
		}
		if !errdefs.IsNotFound(err) {
			return nil, err
		}
	}

	if mode == "never" {
		return nil, fmt.Errorf("image %q is not available", ref)
	}
	var ipath string
	if ipfsPath != nil {
		ipath = *ipfsPath
	}
	r, err := ipfs.NewResolver(ipfs.ResolverOptions{
		Scheme:   scheme,
		IPFSPath: ipath,
	})
	if err != nil {
		return nil, err
	}
	return imgutil.PullImage(ctx, client, stdout, stderr, snapshotter, r, ref, ocispecPlatforms, unpack, quiet)
}

// Push pushes the specified image to IPFS.
func Push(ctx context.Context, client *containerd.Client, rawRef string, layerConvert converter.ConvertFunc, allPlatforms bool, platform []string, ensureImage bool, ipfsPath *string) (string, error) {
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return "", err
	}
	if ensureImage {
		// Ensure image contents are fully downloaded
		logrus.Infof("ensuring image contents")
		if err := ensureContentsOfIPFSImage(ctx, client, rawRef, allPlatforms, platform); err != nil {
			logrus.WithError(err).Warnf("failed to ensure the existence of image %q", rawRef)
		}
	}
	ref, err := referenceutil.ParseAny(rawRef)
	if err != nil {
		return "", err
	}
	return ipfs.PushWithIPFSPath(ctx, client, ref.String(), layerConvert, platMC, ipfsPath)
}

// ensureContentsOfIPFSImage ensures that the entire contents of an existing IPFS image are fully downloaded to containerd.
func ensureContentsOfIPFSImage(ctx context.Context, client *containerd.Client, ref string, allPlatforms bool, platform []string) error {
	platMC, err := platformutil.NewMatchComparer(allPlatforms, platform)
	if err != nil {
		return err
	}
	var img images.Image
	walker := &imagewalker.ImageWalker{
		Client: client,
		OnFound: func(ctx context.Context, found imagewalker.Found) error {
			img = found.Image
			return nil
		},
	}
	n, err := walker.Walk(ctx, ref)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("image does not exist: %q", ref)
	} else if n > 1 {
		return fmt.Errorf("ambiguous reference %q matched %d objects", ref, n)
	}
	cs := client.ContentStore()
	childrenHandler := images.ChildrenHandler(cs)
	childrenHandler = images.SetChildrenLabels(cs, childrenHandler)
	childrenHandler = images.FilterPlatforms(childrenHandler, platMC)
	return images.Dispatch(ctx, images.Handlers(
		remotes.FetchHandler(cs, &fetcher{}),
		childrenHandler,
	), nil, img.Target)
}

// fetcher fetches a file from IPFS
// TODO: fix github.com/containerd/stargz-snapshotter/ipfs to export this and we should import that
type fetcher struct {
}

func (f *fetcher) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	p, err := ipfs.GetCID(desc)
	if err != nil {
		return nil, err
	}
	return ipfsCat(context.TODO(), "ipfs", p, 0, desc.Size, "")
}

func ipfsCat(ctx context.Context, ipfsBin string, c string, off int64, size int64, ipfsPath string) (io.ReadCloser, error) {
	pr, pw := io.Pipe()

	var curCmd *ongoingIPFSCmd
	go func() {
		<-ctx.Done()
		if curCmd != nil {
			discardOngoingIPFSCmd(curCmd)
		}
	}()
	go func() {
		maxretry := 100
		curOff := off
		endOff := off + size
		for i := 0; ; i++ {
			cont, err := func() (cont bool, _ error) { // defer scope
				remain := endOff - curOff
				cmd := exec.Command(ipfsBin, "cat", fmt.Sprintf("--offset=%d", curOff), fmt.Sprintf("--length=%d", remain), c)
				stderrbuf := new(bytes.Buffer)
				cmd.Stderr = stderrbuf
				if ipfsPath != "" {
					cmd.Env = append(os.Environ(), fmt.Sprintf("IPFS_PATH=%s", ipfsPath))
				}
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					return false, err
				}
				if err := cmd.Start(); err != nil {
					return false, err
				}
				go cmd.Wait()

				curCmd = &ongoingIPFSCmd{cmd, stdout, stderrbuf, false}
				defer discardOngoingIPFSCmd(curCmd)

				if n, err := io.CopyN(pw, stdout, remain); err != nil {
					sb, _ := io.ReadAll(stderrbuf)
					if i < maxretry && strings.Contains(string(sb), "someone else has the lock") {
						logrus.WithError(err).WithField("stderr", string(sb)).Warnf("retrying copy %q(offset:%d,length:%d,actuallength:%d,retry:%d/%d)", c, curOff, remain, n, i, maxretry)
						// we need to retry until we can get the lock
						time.Sleep(time.Second)
						curOff += n
						return true, nil
					}
					logrus.WithError(err).WithField("stderr", string(sb)).Warnf("failed to copy %q(offset:%d,length:%d,actuallength:%d,retry:%d/%d)", c, curOff, remain, n, i, maxretry)
					return false, err
				}
				return false, nil
			}()
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			if cont {
				continue
			}
			break
		}
		pw.Close()
	}()
	return pr, nil
}

type ongoingIPFSCmd struct {
	cmd     *exec.Cmd
	outPipe io.ReadCloser
	errBuf  *bytes.Buffer
	done    bool
}

func discardOngoingIPFSCmd(c *ongoingIPFSCmd) {
	if c.done {
		return
	}
	defer func() { c.done = true }()
	// fully read IO until EOF to cleanlly finish the command
	io.Copy(io.Discard, c.outPipe)
	sb, _ := io.ReadAll(c.errBuf)
	time.Sleep(time.Second * 3)
	if !c.cmd.ProcessState.Exited() {
		// kill it if hangs
		logrus.WithField("stderr", string(sb)).Warnf("ipfs command hangs. killing it.")
		c.cmd.Process.Kill()
	}
}
