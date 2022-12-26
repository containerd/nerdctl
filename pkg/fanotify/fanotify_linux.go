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

package fanotify

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/docker/docker/errdefs"
	"github.com/sirupsen/logrus"

	"github.com/containerd/console"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/oci"
	"github.com/containerd/stargz-snapshotter/analyzer/fanotify"
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/spf13/cobra"
)

type Context struct {
	Enable             bool
	Opts               []oci.SpecOpts
	fanotifier         *fanotify.Fanotifier
	accessedFiles      []string
	persistentPath     string
	WaitSignal         bool
	WaitLine           string
	WaitTime           time.Duration
	fanotifierClosed   bool
	fanotifierClosedMu sync.Mutex
}

func GenerateFanotifyOpts(cmd *cobra.Command, flagT bool) (*Context, error) {
	enableFanotify, err := cmd.Flags().GetBool("fanotify")
	if err != nil {
		return nil, fmt.Errorf("failed to get fanotify: %w", err)
	}
	waitTime, err := cmd.Flags().GetInt64("wait-time")
	if err != nil {
		return nil, fmt.Errorf("failed to get wait time: %w", err)
	}
	if waitTime > 0 && !enableFanotify {
		logrus.Warnf("wait-time is unavailable without --fanotify flag")
	}
	waitLine, err := cmd.Flags().GetString("wait-line")
	if err != nil {
		return nil, fmt.Errorf("failed to get wait line: %w", err)
	}
	if waitLine != "" && !enableFanotify {
		logrus.Warnf("wait-line is unavailable without --fanotify flag")
	}
	waitSignal, err := cmd.Flags().GetBool("wait-signal")
	if err != nil {
		return nil, fmt.Errorf("failed to get wait signal: %w", err)
	}
	if waitSignal && !enableFanotify {
		logrus.Warnf("wait-signal is unavailable without --fanotify flag")
	}
	output, err := cmd.Flags().GetString("output")
	if err != nil {
		return nil, fmt.Errorf("failed to get output: %w", err)
	}

	if !enableFanotify {
		return &Context{
			Enable: false,
		}, nil
	}

	if flagT && waitSignal {
		return nil, fmt.Errorf("wait-signal cannot be used with tty option")
	}

	var opts []oci.SpecOpts
	// Spawn a fanotifier process in a new mount namespace.
	fanotifier, err := fanotify.SpawnFanotifier("/proc/self/exe")
	if err != nil {
		return nil, fmt.Errorf("failed to spawn fanotifier: %w", err)
	}
	opts = append(opts, oci.WithLinuxNamespace(runtimespec.LinuxNamespace{
		Type: runtimespec.MountNamespace,
		Path: fanotifier.MountNamespacePath(), // use mount namespace that the fanotifier created
	}))

	return &Context{
		Enable:         true,
		fanotifier:     fanotifier,
		Opts:           opts,
		accessedFiles:  make([]string, 0),
		persistentPath: output,
		WaitTime:       time.Duration(waitTime) * time.Second,
		WaitLine:       waitLine,
		WaitSignal:     waitSignal,
	}, nil
}

func (fanotifyCtx *Context) StartFanotifyMonitor() error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	if !fanotifyCtx.Enable {
		return nil
	}
	if fanotifyCtx.fanotifier == nil {
		return fmt.Errorf("fanotifier is nil")
	}

	if err := fanotifyCtx.fanotifier.Start(); err != nil {
		return fmt.Errorf("failed to start fanotifier: %w", err)
	}

	persistentFd, err := os.Create(fanotifyCtx.persistentPath)
	if err != nil {
		persistentFd.Close()
		return err
	}

	go func() {
		for {
			path, err := fanotifyCtx.fanotifier.GetPath()
			if err != nil {
				if err == io.EOF {
					fanotifyCtx.fanotifierClosedMu.Lock()
					var isFanotifierClosed = fanotifyCtx.fanotifierClosed
					fanotifyCtx.fanotifierClosedMu.Unlock()
					if isFanotifierClosed {
						break
					}
				}
				break
			}
			if !fanotifyCtx.accessedFileExist(path) {
				fmt.Fprintln(persistentFd, path)
				fanotifyCtx.accessedFiles = append(fanotifyCtx.accessedFiles, path)
			}
		}
	}()

	return nil
}

func (fanotifyCtx *Context) NewTask(ctx context.Context, client *containerd.Client, container containerd.Container, taskCtx *TaskContext, flagI, flagT, flagD bool) error {
	if fanotifyCtx.Enable && (fanotifyCtx.WaitSignal || fanotifyCtx.WaitLine != "" || fanotifyCtx.WaitTime > 0) {
		if err := fanotifyCtx.PrepareWaiter(ctx, taskCtx, flagI, flagT); err != nil {
			return err
		}

		task, err := container.NewTask(ctx, taskCtx.IoCreator)
		if err != nil {
			return err
		}
		taskCtx.Task = task
	}

	return nil
}

func (fanotifyCtx *Context) PrepareWaiter(ctx context.Context, taskCtx *TaskContext, flagI, flagT bool) error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	if !fanotifyCtx.Enable {
		return nil
	}

	taskCtx.LineWaiter = &lineWaiter{
		waitCh:         make(chan string),
		waitLineString: fanotifyCtx.WaitLine,
	}
	stdinC := &lazyReadCloser{reader: os.Stdin, initCond: sync.NewCond(&sync.Mutex{})}
	if flagT {
		if !flagI {
			return fmt.Errorf("tty cannot be used if interactive isn't enabled")
		}
		taskCtx.Console = console.Current()
		defer taskCtx.Console.Reset()
		if err := taskCtx.Console.SetRaw(); err != nil {
			return err
		}
		// On tty mode, the "stderr" field is unused.
		taskCtx.IoCreator = cio.NewCreator(cio.WithStreams(taskCtx.Console, taskCtx.LineWaiter.registerWriter(taskCtx.Console), nil), cio.WithTerminal)
	} else {
		if flagI {
			taskCtx.IoCreator = cio.NewCreator(cio.WithStreams(stdinC, taskCtx.LineWaiter.registerWriter(os.Stdout), os.Stderr))
		} else {
			taskCtx.IoCreator = cio.NewCreator(cio.WithStreams(nil, taskCtx.LineWaiter.registerWriter(os.Stdout), os.Stderr))
		}
	}

	stdinC.RegisterCloser(func() { // Ensure to close IO when stdin get EOF
		taskCtx.Task.CloseIO(ctx, containerd.WithStdinCloser)
	})

	return nil
}

func (fanotifyCtx *Context) StartWaiter(ctx context.Context, container containerd.Container, taskCtx *TaskContext, flagD, rm bool) error {
	if fanotifyCtx == nil {
		return fmt.Errorf("fanotifyCtx is nil")
	}
	if taskCtx == nil {
		return fmt.Errorf("taskCtx is nil")
	}
	var err error
	var code uint32
	if fanotifyCtx.Enable && (fanotifyCtx.WaitSignal || fanotifyCtx.WaitLine != "" || fanotifyCtx.WaitTime > 0) {
		var status containerd.ExitStatus
		var killOk bool
		// Wait until the task exit
		if fanotifyCtx.WaitSignal { // NOTE: not functional with `tty` option
			logrus.Infoln("press Ctrl+C to terminate the container")
			status, killOk, err = waitSignalHandler(ctx, container, taskCtx.Task)
			if err != nil {
				return err
			}
		} else {
			if fanotifyCtx.WaitLine != "" {
				logrus.Infof("waiting for line \"%v\" ...", fanotifyCtx.WaitLine)
				status, killOk, err = waitLineHandler(ctx, container, taskCtx.Task, taskCtx.LineWaiter)
				if err != nil {
					return err
				}
			} else {
				logrus.Infof("waiting for %v ...", fanotifyCtx.WaitTime)
				status, killOk, err = waitTimeHandler(ctx, container, taskCtx.Task, fanotifyCtx.WaitTime)
				if err != nil {
					return err
				}
			}
		}
		if !killOk {
			logrus.Warnf("failed to exit task %v; manually kill it", taskCtx.Task.ID())
		} else {
			code, _, err = status.Result()
			if err != nil {
				return err
			}
			logrus.Infof("container exit with code %v", code)
			if _, err := taskCtx.Task.Delete(ctx); err != nil {
				return err
			}
		}

		fanotifyCtx.fanotifierClosedMu.Lock()
		fanotifyCtx.fanotifierClosed = true
		fanotifyCtx.fanotifierClosedMu.Unlock()

		if fanotifyCtx.fanotifier != nil {
			if err := fanotifyCtx.fanotifier.Close(); err != nil {
				return fmt.Errorf("failed to cleanup fanotifier")
			}
		}
	} else {
		var statusC <-chan containerd.ExitStatus
		if !flagD {
			defer func() {
				if rm {
					if _, taskDeleteErr := taskCtx.Task.Delete(ctx); taskDeleteErr != nil {
						logrus.Error(taskDeleteErr)
					}
				}
			}()
			statusC, err = taskCtx.Task.Wait(ctx)
			if err != nil {
				return err
			}
		}

		status := <-statusC
		code, _, err = status.Result()
		if err != nil {
			return err
		}
	}
	taskCtx.ExitCode = code
	return nil
}

func (fanotifyCtx *Context) accessedFileExist(filePath string) bool {
	tmpAccessedFiles := make([]string, len(fanotifyCtx.accessedFiles))
	copy(tmpAccessedFiles, fanotifyCtx.accessedFiles)
	sort.Strings(tmpAccessedFiles)
	if index := sort.SearchStrings(tmpAccessedFiles, filePath); index < len(tmpAccessedFiles) && tmpAccessedFiles[index] == filePath {
		return true
	}
	return false
}

func (s *lazyReadCloser) RegisterCloser(closer func()) {
	s.closerMutex.Lock()
	s.closer = closer
	s.closerMutex.Unlock()
	atomic.AddInt64(&s.initialized, 1)
	s.initCond.Broadcast()
}
func (s *lazyReadCloser) Read(p []byte) (int, error) {
	if atomic.LoadInt64(&s.initialized) <= 0 {
		// wait until initialized
		s.initCond.L.Lock()
		if atomic.LoadInt64(&s.initialized) <= 0 {
			s.initCond.Wait()
		}
		s.initCond.L.Unlock()
	}

	n, err := s.reader.Read(p)
	if err == io.EOF {
		s.closerMutex.Lock()
		s.closer()
		s.closerMutex.Unlock()
	}
	return n, err
}

func (lw *lineWaiter) registerWriter(w io.Writer) io.Writer {
	if lw.waitLineString == "" {
		return w
	}

	pr, pw := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), lw.waitLineString) {
				lw.waitCh <- lw.waitLineString
			}
		}
		if _, err := io.Copy(io.Discard, pr); err != nil {
			pr.CloseWithError(err)
			return
		}
	}()

	return io.MultiWriter(w, pw)
}

func waitSignalHandler(ctx context.Context, container containerd.Container, task containerd.Task) (containerd.ExitStatus, bool, error) {
	statusC, err := task.Wait(ctx)
	if err != nil {
		return containerd.ExitStatus{}, false, err
	}
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT)
	defer signal.Stop(sc)
	select {
	case status := <-statusC:
		return status, true, nil
	case <-sc:
		logrus.Infoln("signal detected")
		status, err := killTask(ctx, container, task, statusC)
		if err != nil {
			logrus.Errorln("failed to kill container")
			return containerd.ExitStatus{}, false, err
		}
		return status, true, nil
	}
}

func waitTimeHandler(ctx context.Context, container containerd.Container, task containerd.Task, waitTime time.Duration) (containerd.ExitStatus, bool, error) {
	statusC, err := task.Wait(ctx)
	if err != nil {
		return containerd.ExitStatus{}, false, err
	}
	select {
	case status := <-statusC:
		return status, true, nil
	case <-time.After(waitTime):
		logrus.Warnf("killing task. the wait time (%s) reached", waitTime.String())
	}
	status, err := killTask(ctx, container, task, statusC)
	if err != nil {
		logrus.Warnln("failed to kill container")
		return containerd.ExitStatus{}, false, err
	}
	return status, true, nil
}

func waitLineHandler(ctx context.Context, container containerd.Container, task containerd.Task, waitLine *lineWaiter) (containerd.ExitStatus, bool, error) {
	if waitLine == nil {
		return containerd.ExitStatus{}, false, fmt.Errorf("lineWaiter is nil")
	}

	statusC, err := task.Wait(ctx)
	if err != nil {
		return containerd.ExitStatus{}, false, err
	}
	select {
	case status := <-statusC:
		return status, true, nil
	case l := <-waitLine.waitCh:
		logrus.Infof("Waiting line detected %q, killing task", l)
	}
	status, err := killTask(ctx, container, task, statusC)
	if err != nil {
		logrus.Warnln("failed to kill container")
		return containerd.ExitStatus{}, false, err
	}
	return status, true, nil
}

func killTask(ctx context.Context, container containerd.Container, task containerd.Task, statusC <-chan containerd.ExitStatus) (containerd.ExitStatus, error) {
	sig, err := containerd.GetStopSignal(ctx, container, syscall.SIGKILL)
	if err != nil {
		return containerd.ExitStatus{}, err
	}
	if err := task.Kill(ctx, sig, containerd.WithKillAll); err != nil && !errdefs.IsNotFound(err) {
		return containerd.ExitStatus{}, fmt.Errorf("forward SIGKILL: %w", err)
	}
	select {
	case status := <-statusC:
		return status, nil
	case <-time.After(5 * time.Second):
		return containerd.ExitStatus{}, fmt.Errorf("forward SIGKILL: %w", err)
	}
}
