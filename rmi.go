/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/errdefs"
	"github.com/containerd/containerd/images"
	"github.com/containerd/containerd/platforms"
	"github.com/opencontainers/go-digest"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
)

var rmiCommand = &cli.Command{
	Name:      "rmi",
	Usage:     "Remove one or more images",
	ArgsUsage: "[flags] IMAGE [IMAGE, ...]",
	Action:    rmiAction,
}

func rmiAction(clicontext *cli.Context) error {
	if clicontext.NArg() == 0 {
		return errors.Errorf("requires at least 1 argument")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()

	var (
		imageStore = client.ImageService()
		cs         = client.ContentStore()
	)

	imageList, err := imageStore.List(ctx, "")
	if err != nil {
		return err
	}

	var opts []images.DeleteOpt
	var imageNotFoundError bool

	if clicontext.NArg() >= 1 {
		img := clicontext.Args().First()
		imgFQIN := getFQIN(img)

		digests, err := getImageDigests(ctx, cs, imgFQIN, imageList)
		if err != nil {
			return errors.Errorf("Error in getting image digests: %v", err)
		}

		if err := imageStore.Delete(ctx, imgFQIN, opts...); err != nil {
			fmt.Printf("Error: No such image: %s\n", img)
			imageNotFoundError = true
		} else {
			printDigests(imgFQIN, digests)
		}
	}

	if clicontext.NArg() == 1 {
		if imageNotFoundError {
			os.Exit(1)
		}
		os.Exit(0)
	}

	for _, img := range clicontext.Args().Tail() {
		imgFQIN := getFQIN(img)

		digests, err := getImageDigests(ctx, cs, imgFQIN, imageList)
		if err != nil {
			return errors.Errorf("Error in getting image digests: %v", err)
		}

		if err := imageStore.Delete(ctx, imgFQIN, opts...); err != nil {
			if errdefs.IsNotFound(err) {
				fmt.Printf("Error: No such image: %s\n", img)
				imageNotFoundError = true
				continue
			}
			return err
		}
		printDigests(imgFQIN, digests)
	}

	if imageNotFoundError {
		os.Exit(1)
	}

	return nil
}

// Print digests after image removal.
// This will keep the stdout in sync with docker rmi output.
func printDigests(imgFQIN string, digests []digest.Digest) {
	if strings.Contains(imgFQIN, "docker.io/library") {
		imgFQIN = imgFQIN[18:]
	}
	fmt.Printf("Untagged: %s\n", imgFQIN)
	for _, digest := range digests {
		fmt.Printf("Deleted: %s\n", digest)
	}
}

// Get SHA digests for the given image.
func getImageDigests(ctx context.Context, cs content.Store, imgFQIN string, imageList []images.Image) ([]digest.Digest, error) {
	var digests []digest.Digest
	var err error
	for _, image := range imageList {
		if imgFQIN == image.Name {
			digests, err = image.RootFS(ctx, cs, platforms.Default())
			if err != nil {
				return nil, err
			}
			break
		}
	}
	return digests, nil
}

// Get fully qualified image name (FQIN) for the given image.
func getFQIN(img string) string {
	if !strings.Contains(img, "/") {
		img = "docker.io/library/" + img
	}

	if !strings.Contains(img, ":") {
		img = img + ":latest"
	}
	return img
}
