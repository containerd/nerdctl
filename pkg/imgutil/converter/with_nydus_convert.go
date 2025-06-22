//go:build !no_nydus

package converter

import (
	"github.com/containerd/containerd/v2/core/images/converter"
	nydusconvert "github.com/containerd/nydus-snapshotter/pkg/converter"
	"github.com/containerd/platforms"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
)

func NydusConvertOpt(options types.NydusOptions, platMC platforms.MatchComparer, defaultWorkDir string) (converter.Opt, error) {
	workDir := options.NydusWorkDir
	if workDir == "" {
		workDir = defaultWorkDir
	}

	nydusOpts := getNydusConvertOpts(options, workDir)
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
	return converter.WithIndexConvertFunc(
		converter.IndexConvertFuncWithHook(
			nydusconvert.LayerConvertFunc(*nydusOpts),
			true,
			platMC,
			convertHooks,
		)), nil
}

func getNydusConvertOpts(options types.NydusOptions, workDir string) *nydusconvert.PackOption {
	return &nydusconvert.PackOption{
		BuilderPath: options.NydusBuilderPath,
		// the path will finally be used is <NERDCTL_DATA_ROOT>/nydus-converter-<hash>,
		// for example: /var/lib/nerdctl/1935db59/nydus-converter-3269662176/,
		// and it will be deleted after the conversion
		WorkDir:          workDir,
		PrefetchPatterns: options.NydusPrefetchPatterns,
		Compressor:       options.NydusCompressor,
		FsVersion:        "6",
	}
}
