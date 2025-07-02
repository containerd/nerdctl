//go:build !no_nydus

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

package image

import (
	"context"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/images/converter"
	"github.com/containerd/platforms"
	nydusconvert "github.com/containerd/nydus-snapshotter/pkg/converter"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
)

func getNydusConvertOpts(options types.ImageConvertOptions) (*nydusconvert.PackOption, error) {
	workDir := options.NydusWorkDir
	if workDir == "" {
		var err error
		workDir, err = clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
		if err != nil {
			return nil, err
		}
	}
	return &nydusconvert.PackOption{
		BuilderPath: options.NydusBuilderPath,
		// the path will finally be used is <NERDCTL_DATA_ROOT>/nydus-converter-<hash>,
		// for example: /var/lib/nerdctl/1935db59/nydus-converter-3269662176/,
		WorkDir:          workDir + "/nydus-converter-%v/",
		PrefetchPatterns: options.NydusPrefetchPatterns,
		Compressor:       options.NydusCompressor,
		FsVersion:        "6",
	}, nil
}

func addNydusConverterOpts(ctx context.Context, client *containerd.Client, options types.ImageConvertOptions, convertOpts []converter.Opt, platMC platforms.MatchComparer) ([]converter.Opt, error) {
	nydusOpts, err := getNydusConvertOpts(options)
	if err != nil {
		return convertOpts, err
	}
	convertHooks := converter.ConvertHooks{
		PostConvertHook: nydusconvert.ConvertHookFunc(nydusconvert.MergeOption{
			WorkDir:          nydusOpts.WorkDir,
			BuilderPath:      nydusOpts.BuilderPath,
			FsVersion:        nydusOpts.FsVersion,
			ChunkDictPath:    nydusOpts.ChunkDictPath,
			PrefetchPatterns: nydusOpts.PrefetchPatterns,
			OCI:              true,
		}),
	}
	convertOpts = append(convertOpts, converter.WithIndexConvertFunc(
		converter.IndexConvertFuncWithHook(
			nydusconvert.LayerConvertFunc(*nydusOpts),
			true,
			platMC,
			convertHooks,
		),
	))
	return convertOpts, nil
}