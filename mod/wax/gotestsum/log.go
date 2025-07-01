package gotestsum

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gotest.tools/gotestsum/testjson"
)

func FromLogs(files ...string) (run *Run, err error) {
	run = &Run{
		Pipelines: []*Pipeline{},
	}

	for _, file := range files {
		in, err := jsonlogReader(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read jsonfile: %v", err)
		}

		defer func() {
			err = errors.Join(in.Close(), err)
		}()

		exec, err := testjson.ScanTestOutput(testjson.ScanConfig{Stdout: in})
		if err != nil {
			return nil, fmt.Errorf("failed to scan testjson: %v", err)
		}

		pipeline := &Pipeline{}
		pkgs := exec.Packages()
		for _, pkg := range pkgs {
			pipeline.Packages = append(pipeline.Packages, exec.Package(pkg))
		}

		run.Pipelines = append(run.Pipelines, pipeline)
	}

	return run, nil
}

func FindLogs(root, needle string) []string {
	return findFiles(root, needle)
}

func jsonlogReader(v string) (io.ReadCloser, error) {
	switch v {
	case "", "-":
		return io.NopCloser(os.Stdin), nil
	default:
		return os.Open(v)
	}
}
