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

package main

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/nerdctl/pkg/copyutil"
	"github.com/containerd/nerdctl/pkg/idutil/containerwalker"
	securejoin "github.com/cyphar/filepath-securejoin"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type cpConfig struct {
	container     string
	containerPid  uint32
	containerUser runtimespec.User
	followLink    bool
	sourcePath    string
	destPath      string
}

type TarPipe struct {
	src       string
	dst       string
	option    *cpConfig
	reader    *io.PipeReader
	outStream *io.PipeWriter
	readChan  chan uint64
}

var errFileSpecDoesntMatchFormat = errors.New("filespec must match the canonical format: [container:]file/path")

func newCpCommand() *cobra.Command {

	shortHelp := "Copy files/folders between a running container and the local filesystem"

	longHelp := shortHelp + `
    WARNING: This command is not designed to be used with untrusted containers.
    NOTE: Use '-' as the source to read a tar archive from stdin
    and extract it to a directory destination in a container.
    Use '-' as the destination to stream a tar archive of a\ncontainer source to stdout.
    `

	usage := `cp [OPTIONS] CONTAINER:SRC_PATH DEST_PATH|-
  nerdctl cp [OPTIONS] SRC_PATH|- CONTAINER:DEST_PATH`
	var cpCommand = &cobra.Command{
		Use:               usage,
		Args:              cobra.ExactArgs(2),
		Short:             shortHelp,
		Long:              longHelp,
		RunE:              cpAction,
		ValidArgsFunction: cpShellComplete,
		SilenceUsage:      true,
		SilenceErrors:     true,
	}

	cpCommand.Flags().BoolP("follow-link", "L", false, "Always follow symbol link in SRC_PATH.")
	cpCommand.Flags().BoolP("archive", "a", false, "Archive mode (copy all uid/gid information).")

	return cpCommand
}

func cpAction(cmd *cobra.Command, args []string) error {

	if runtime.GOOS == "windows" {
		return fmt.Errorf("cp not yet supported for windows platform")
	}

	srcSpec, err := splitCpArg(args[0])
	if err != nil {
		return err
	}

	destSpec, err := splitCpArg(args[1])
	if err != nil {
		return err
	}

	flagL, err := cmd.Flags().GetBool("follow-link")
	if err != nil {
		return err
	}

	copyConfig := cpConfig{
		followLink: flagL,
		sourcePath: srcSpec.Path,
		destPath:   destSpec.Path,
	}

	if len(srcSpec.Container) != 0 && len(destSpec.Container) != 0 {
		return fmt.Errorf("one of src or dest must be a local file specification")
	}
	if len(srcSpec.Path) == 0 || len(destSpec.Path) == 0 {
		return errors.New("filepath can not be empty")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()

	// Supporting only running containers for the moment
	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			container, err := client.LoadContainer(ctx, found.Container.ID())
			if err != nil {
				return err
			}

			spec, err := container.Spec(ctx)
			if err != nil {
				return err
			}
			copyConfig.containerUser = spec.Process.User

			task, err := container.Task(ctx, cio.Load)
			if err != nil {
				return err
			}
			copyConfig.containerPid = task.Pid()

			status, err := task.Status(ctx)
			if err != nil {
				return err
			}

			switch status.Status {
			case containerd.Running:
				if len(srcSpec.Container) != 0 {
					return copyFromContainer(ctx, cmd, copyConfig)
				}
				if len(destSpec.Container) != 0 {
					return copyToContainer(ctx, cmd, copyConfig)
				}

				return fmt.Errorf("one of src or dest must be a remote file specification")
			case containerd.Stopped, containerd.Pausing, containerd.Created, containerd.Paused:
				return fmt.Errorf("We only support Running Container")
			default:
			}
			return nil
		},
	}

	if len(srcSpec.Container) != 0 {
		copyConfig.container = srcSpec.Container
	}
	if len(destSpec.Container) != 0 {
		copyConfig.container = destSpec.Container
	}

	n, err := walker.Walk(ctx, copyConfig.container)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", copyConfig.container)
	}

	return nil
}

func splitCpArg(arg string) (fileSpec, error) {
	i := strings.Index(arg, ":")

	// filespec starting with a semicolon is invalid
	if i == 0 {
		return fileSpec{}, errFileSpecDoesntMatchFormat
	}

	if filepath.IsAbs(arg) {
		// Explicit local absolute path, e.g., `C:\foo` or `/foo`.
		return fileSpec{
			Path: arg,
		}, nil
	}

	parts := strings.SplitN(arg, ":", 2)

	if len(parts) == 1 || strings.HasPrefix(parts[0], ".") {
		// Either there's no `:` in the arg
		// OR it's an explicit local relative path like `./file:name.txt`.
		return fileSpec{
			Path: arg,
		}, nil
	}

	return fileSpec{
		Container: parts[0],
		Path:      parts[1],
	}, nil
}

type fileSpec struct {
	Container string
	Path      string
}

func copyToContainer(ctx context.Context, cmd *cobra.Command, copyConfig cpConfig) (err error) {

	// Init tar Pipe is the first step to process copy to container
	t := initTarPipe(&copyConfig)

	if _, err := os.Stat(t.src); err != nil {
		return fmt.Errorf("%s doesn't exist in local filesystem", t.src)
	}

	// TarPipe destination should exist in the container and it should be a directory
	destination, err := securejoin.SecureJoin(filepath.Join("/proc/", fmt.Sprint(t.option.containerPid), "/root"), t.dst)
	if err != nil {
		return err
	}

	d, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("%s should exists", t.dst)
	} else {
		if !d.IsDir() {
			return fmt.Errorf("%s should be a directory", t.dst)
		}
	}

	go func(*TarPipe) {
		defer t.outStream.Close()
		makeTar(cmd, t)
	}(t)

	// check if tar binary exists in the host
	tarBinary, err := copyutil.TarBinary()
	if err != nil {
		hint := fmt.Sprintf("`tar` binary needs to be installed")
		return fmt.Errorf(hint+": %w", err)
	}

	var tarArgs []string
	tarArgs = []string{"-xmf", "-"}

	dstDir, _ := copyutil.SplitPathDirEntry(filepath.Clean(t.dst))
	var target string
	if len(dstDir) > 0 {
		target, err = securejoin.SecureJoin(filepath.Join("/proc/", fmt.Sprint(t.option.containerPid), "/root"), dstDir)
		if err != nil {
			return err
		}
	} else {
		target, err = securejoin.SecureJoin(filepath.Join("/proc/", fmt.Sprint(t.option.containerPid), "/root"), "")
		if err != nil {
			return err
		}
	}

	tarArgs = append(tarArgs, "-C", target)
	command := exec.Command(tarBinary, tarArgs...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return err
	}

	go func(*TarPipe) error {

		defer stdin.Close()

		writer := io.Writer(stdin)
		if _, err := io.Copy(writer, t.reader); err != nil {
			return err
		}

		return nil
	}(t)

	out, err := command.CombinedOutput()
	if err != nil {
		logrus.Errorf("failed to exec: %v: %v", string(out), err)
	}

	return nil
}

func makeTar(cmd *cobra.Command, t *TarPipe) error {

	// making Tar flow:
	// tar --> buf

	tw := tar.NewWriter(t.outStream)
	defer tw.Close()

	srcDir, srcBase := copyutil.SplitPathDirEntry(filepath.Clean(t.src))
	dstDir, dstBase := copyutil.SplitPathDirEntry(filepath.Clean(t.dst))

	// At this point, we consider that destination exists and it is a directory
	// Override dstBase by copying specified src into it
	dstBase = filepath.Join(dstBase, srcBase)

	archive, err := cmd.Flags().GetBool("archive")
	if err != nil {
		return err
	}

	return recursiveTar(srcDir, srcBase, dstDir, dstBase, t.option.containerUser, tw, archive)
}

// recursiveTar makes a tar recursively
func recursiveTar(srcDir, srcBase, dstDir, dstBase string, user runtimespec.User, tw *tar.Writer, archive bool) error {
	matchedPaths, err := filepath.Glob(filepath.Join(srcDir, srcBase))
	if err != nil {
		return err
	}

	for _, path := range matchedPaths {
		stat, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if stat.IsDir() {
			files, err := os.ReadDir(path)
			if err != nil {
				return err
			}
			hdr, _ := tar.FileInfoHeader(stat, path)

			if archive {
				hdr.Gid = int(user.GID)
				hdr.Uid = int(user.UID)
				hdr.Uname = user.Username
				// Gname will be set with GIDMapping when extracting the tar
				hdr.Gname = ""
			}

			//case empty directory
			if len(files) == 0 {
				hdr.Name = dstBase

				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}
			}

			for _, f := range files {
				if err := recursiveTar(srcDir, filepath.Join(srcBase, copyutil.StripTrailingSlash(f.Name())),
					dstDir, filepath.Join(dstBase, copyutil.StripTrailingSlash(f.Name())), user, tw, archive); err != nil {
					return err
				}
			}
			return nil
		} else if stat.Mode()&os.ModeSymlink != 0 {
			//case soft link
			hdr, _ := tar.FileInfoHeader(stat, path)
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}

			hdr.Linkname = target
			hdr.Name = dstBase
			if archive {
				hdr.Gid = int(user.GID)
				hdr.Uid = int(user.UID)
				hdr.Uname = user.Username
				// Gname will be set with GIDMapping when extracting the tar
				hdr.Gname = ""
			}
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
		} else {
			//case regular file or other file type like pipe
			hdr, err := tar.FileInfoHeader(stat, path)
			if err != nil {
				return err
			}
			hdr.Name = dstBase
			if archive {
				hdr.Gid = int(user.GID)
				hdr.Uid = int(user.UID)
				hdr.Uname = user.Username
				// Gname will be set with GIDMapping when extracting the tar
				hdr.Gname = ""
			}

			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}

			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(tw, f); err != nil {
				return err
			}

			return f.Close()
		}
		return nil
	}

	return nil
}
func copyFromContainer(ctx context.Context, cmd *cobra.Command, copyConfig cpConfig) (err error) {
	dstPath := copyConfig.destPath
	srcPath := copyConfig.sourcePath

	t := initTarPipe(&copyConfig)

	t.startReadFrom(0)

	tarReader := tar.NewReader(t)

	for {
		header, err := tarReader.Next()
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
		// basic file information
		mode := header.FileInfo().Mode()

		if copyConfig.followLink {
			if mode&os.ModeSymlink != 0 {
				linkTarget := header.Linkname
				srcDir, _ := copyutil.SplitPathDirEntry(srcPath)
				if !filepath.IsAbs(linkTarget) {
					// Join with the parent directory.
					linkTarget = filepath.Join(srcDir, linkTarget)
				}

				if copyutil.IsCurrentDir(srcDir) &&
					!copyutil.IsCurrentDir(linkTarget) {
					linkTarget += string(filepath.Separator) + "."
				}

				if copyutil.HasTrailingPathSeparator(srcDir, string(os.PathSeparator)) &&
					!copyutil.HasTrailingPathSeparator(linkTarget, string(os.PathSeparator)) {
					linkTarget += string(filepath.Separator)
				}

				srcPath = linkTarget
			}
		}

		// srcPath Cleaning
		trimmedPath := strings.TrimLeft(srcPath, `/\`) // tar strips the leading '/' and '\' if it's there, so we will too
		cleanedPath := filepath.Clean(trimmedPath)
		prefix := copyutil.StripPathShortcuts(cleanedPath)

		// header.Name is a name of the REMOTE file, so we need to create
		// a remotePath that will goes through appropriate cleaning process
		destFileName := path.Join(dstPath, filepath.Clean(header.Name[len(prefix):]))

		if !copyutil.IsRelative(dstPath, destFileName) {
			logrus.Warnf("skipping %q: file is outside target destination", destFileName)
			continue
		}

		// docker does not manage the creation of non-existent path
		if err := os.MkdirAll(filepath.Dir(destFileName), 0755); err != nil {
			return err
		}
		if header.FileInfo().IsDir() {
			if err := os.MkdirAll(destFileName, 0755); err != nil {
				return err
			}
			continue
		}

		if dstPath == "-" {
			_, err = io.Copy(cmd.OutOrStdout(), tarReader)
			return err
		}

		outFile, err := os.Create(destFileName)
		if err != nil {
			return err
		}
		defer func() {
			if err := outFile.Close(); err != nil {
				logrus.WithError(err).Warnf("failed to close: %q", outFile.Name())
			}
		}()

		writer := io.Writer(outFile)
		if _, err := io.Copy(writer, tarReader); err != nil {
			return err
		}

	}

	return nil
}

func (t *TarPipe) startReadFrom(n uint64) {
	tarBinary, err := copyutil.TarBinary()
	if err != nil {
		hint := fmt.Sprintf("`tar` binary needs to be installed")
		logrus.Errorf(hint+": %w", err)
	}

	var tarArgs []string
	tarArgs = []string{"-cf", "-", "-C", filepath.Join("/proc/", fmt.Sprint(t.option.containerPid), "/root"), strings.TrimLeft(t.src, `/\`)}

	command := exec.Command(tarBinary, tarArgs...)
	command.Stdout = t.outStream

	go func() {
		defer t.outStream.Close()
		err = command.Run()
		if err != nil {
			logrus.Errorf("failed to exec: %v", err)
		}
	}()
}

func (t *TarPipe) Read(p []byte) (n int, err error) {
	n, err = t.reader.Read(p)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func initTarPipe(copyConfig *cpConfig) *TarPipe {
	t := TarPipe{
		src:      copyConfig.sourcePath,
		dst:      copyConfig.destPath,
		option:   copyConfig,
		readChan: make(chan uint64), // value: byte number
	}
	t.reader, t.outStream = io.Pipe()

	return &t
}

func cpShellComplete(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterFileExt
}
