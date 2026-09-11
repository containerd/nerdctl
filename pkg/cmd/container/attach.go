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

package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"golang.org/x/term"

	"github.com/containerd/console"
	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/attachmux"
	"github.com/containerd/nerdctl/v2/pkg/consoleutil"
	"github.com/containerd/nerdctl/v2/pkg/containerutil"
	"github.com/containerd/nerdctl/v2/pkg/errutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/logging/loguri"
	"github.com/containerd/nerdctl/v2/pkg/signalutil"
)

// Attach attaches stdin, stdout, and stderr to a running container.
func Attach(ctx context.Context, client *containerd.Client, req string, options types.ContainerAttachOptions) error {
	// Find the container.
	var container containerd.Container
	var cStatus containerd.Status

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			container = found.Container
			return nil
		},
	}
	n, err := walker.Walk(ctx, req)
	if err != nil {
		return fmt.Errorf("error when trying to find the container: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("no container is found given the string: %s", req)
	} else if n > 1 {
		return fmt.Errorf("more than one containers are found given the string: %s", req)
	}

	defer func() {
		containerLabels, err := container.Labels(ctx)
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to getting container labels: %s", err)
			return
		}
		rm, err := containerutil.DecodeContainerRmOptLabel(containerLabels[labels.ContainerAutoRemove])
		if err != nil {
			log.G(ctx).WithError(err).Errorf("failed to decode string to bool value: %s", err)
			return
		}
		if rm && cStatus.Status == containerd.Stopped {
			if err = RemoveContainer(ctx, container, options.GOptions, true, true, client); err != nil {
				log.L.WithError(err).Warnf("failed to remove container %s: %s", req, err)
			}
		}
	}()

	// Attach to the container.
	var task containerd.Task
	detachC := make(chan struct{}, 1)
	spec, err := container.Spec(ctx)
	if err != nil {
		return fmt.Errorf("failed to get the OCI runtime spec for the container: %w", err)
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()

	// Detaching means different things on the two paths below: ending this
	// socket session, or cancelling the FIFO copy and telling the select at the
	// end of this function. Which path applies is only known once containerd has
	// reported how the task's stdio is wired, and the stdin reader has to exist
	// before that, because it is part of the cio.Attach the legacy path builds.
	// So the closer does both: cancelStream is a no-op on the legacy path, and
	// the send is dropped on the socket path, which does not read detachC.
	//
	// It deliberately does not touch task. cio.NewAttach starts copying stdin
	// from inside its own callback, before container.Task returns and assigns
	// task (pkg/cio/io_unix.go, copyIO), so a user who hits the detach keys
	// immediately would reach a nil task here. Upstream gets away with the same
	// shape only because its unbuffered send blocks until the select below has
	// been reached; cancelling the task IO is done there instead.
	closer := func() {
		cancelStream()
		select {
		case detachC <- struct{}{}:
		default:
		}
	}

	var (
		opt    cio.Opt
		con    console.Console
		stdin  io.Reader
		stdout = options.Stdout
		stderr = options.Stderr
	)
	if spec.Process.Terminal {
		con, err = consoleutil.Current()
		if err != nil {
			return err
		}
		defer con.Reset()

		if _, err := term.MakeRaw(int(con.Fd())); err != nil {
			return fmt.Errorf("failed to set the console to raw mode: %w", err)
		}
		// A terminal container has a single output stream.
		stdout, stderr = con, nil
		if options.Stdin != nil {
			stdin, err = consoleutil.NewDetachableStdin(con, options.DetachKeys, closer)
			if err != nil {
				return err
			}
		}
	} else {
		stdin = options.Stdin
	}
	opt = cio.WithStreams(stdin, stdout, stderr)

	// Ask containerd how the task's stdio is wired, and build the IO for it in
	// the same call. A URI in Stdout means the output goes to a process spawned
	// from the log URI, and cio.NewAttach would hand that URI to fifo.OpenFifo
	// as a path; a plain path means FIFOs this process can open. Classifying in
	// a separate TaskService.Get first would leave a window in which containerd's
	// restart monitor replaces a FIFO task with a URI one between the two calls.
	//
	// NullIO registers no closers, so the handle returned for the socket path
	// serves task.Wait and friends and leaves the container's FIFOs alone. The
	// callback is skipped for a task in an unknown state, which has no stdio to
	// attach to either way.
	var brokerURI string
	var sawIO bool
	task, err = container.Task(ctx, func(fifos *cio.FIFOSet) (cio.IO, error) {
		sawIO = true
		if u, uerr := url.Parse(fifos.Stdout); uerr == nil && u.Scheme != "" {
			brokerURI = fifos.Stdout
			return cio.NullIO("")
		}
		return cio.NewAttach(opt)(fifos)
	})
	if err != nil {
		return fmt.Errorf("failed to attach to the container: %w", err)
	}
	if !sawIO {
		return fmt.Errorf("the task of container %s has no stdio to attach to", req)
	}

	if brokerURI != "" {
		// The data store comes out of the URI itself, which is the string the
		// logging process was handed in its argv and derives the socket path
		// from. Resolving it again here could differ over a symlinked
		// --data-root.
		dataStore := loguri.DataStore(brokerURI)
		if dataStore == "" || !loguri.IsInternal(brokerURI) {
			return fmt.Errorf("cannot attach to container %s: its stdio is owned by %q", req, brokerURI)
		}
		socketPath := attachmux.SocketPath(dataStore, options.GOptions.Namespace, container.ID())
		cStatus, err = attachSession(ctx, streamCtx, socketPath, task, spec, stdin, stdout, stderr)
		return err
	}

	log.G(ctx).Debug("attaching to the container stdio directly: only one session is supported")
	if spec.Process.Terminal {
		if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
			log.G(ctx).WithError(err).Error("console resize")
		}
	}
	sigC := signalutil.ForwardAllSignals(ctx, task)
	defer signalutil.StopCatch(sigC)

	// Wait for the container to exit.
	statusC, err := task.Wait(ctx)
	if err != nil {
		return fmt.Errorf("failed to init an async wait for the container to exit: %w", err)
	}
	select {
	// io.Wait() would return when either 1) the user detaches from the container OR 2) the container is about to exit.
	//
	// If we replace the `select` block with io.Wait() and
	// directly use task.Status() to check the status of the container after io.Wait() returns,
	// it can still be running even though the container is about to exit (somehow especially for Windows).
	//
	// As a result, we need a separate detachC to distinguish from the 2 cases mentioned above.
	case <-detachC:
		io := task.IO()
		if io == nil {
			return errors.New("got a nil IO from the task")
		}
		// Cancel used to live in the closer, where it raced with the assignment
		// of task above.
		io.Cancel()
		io.Wait()
	case status := <-statusC:
		cStatus, err = task.Status(ctx)
		if err != nil {
			return err
		}
		code, _, err := status.Result()
		if err != nil {
			return err
		}
		if code != 0 {
			return errutil.NewExitCoderErr(int(code))
		}
	}
	return nil
}

// attachSession runs an attach session over the container's attach socket. Any
// number of sessions can be connected at once: the process owning the
// container's stdio fans its output out to all of them.
//
// task is the handle obtained while reading the task's IO configuration. Its
// own IO is a no-op, which is all this path needs: output arrives over the
// socket, and the task is used only for Wait, Status, Resize and signals.
// Detaching ends streamCtx, which closes this session's connection and leaves
// the container, and every other session, untouched.
func attachSession(
	ctx, streamCtx context.Context,
	socketPath string,
	task containerd.Task,
	spec *oci.Spec,
	stdin io.Reader,
	stdout, stderr io.Writer,
) (containerd.Status, error) {
	var cStatus containerd.Status

	session, err := attachmux.Dial(ctx, socketPath)
	if err != nil {
		return cStatus, fmt.Errorf("failed to connect to the attach socket %q of the container: %w", socketPath, err)
	}
	defer session.Close()

	if spec.Process.Terminal {
		if con, ok := stdout.(console.Console); ok {
			if err := consoleutil.HandleConsoleResize(ctx, task, con); err != nil {
				log.G(ctx).WithError(err).Error("console resize")
			}
		}
	}
	sigC := signalutil.ForwardAllSignals(ctx, task)
	defer signalutil.StopCatch(sigC)

	statusC, err := task.Wait(ctx)
	if err != nil {
		return cStatus, fmt.Errorf("failed to init an async wait for the container to exit: %w", err)
	}

	streamed := make(chan error, 1)
	go func() { streamed <- session.Stream(streamCtx, stdin, stdout, stderr) }()

	select {
	case err := <-streamed:
		if err != nil {
			// The container may have exited at the same instant. Both channels
			// are then ready and the select above picks either, so this branch
			// has to reach the same answer the other one does.
			select {
			case status := <-statusC:
				return exitResult(ctx, task, status, err)
			default:
			}
			return cStatus, fmt.Errorf("the container attach session failed: %w", err)
		}
		if !session.Exited() {
			// The user detached. The container keeps running, and any other
			// session stays attached.
			return cStatus, nil
		}
		// The broker says the container is gone. Its exit code comes from
		// containerd, never from the broker, so that there is one source of
		// truth for it. The wait is bounded: if containerd disagrees, the
		// session was wrong about the exit and this must not hang.
		select {
		case status := <-statusC:
			return exitResult(ctx, task, status, nil)
		case <-time.After(attachmux.DrainTimeout):
			return cStatus, errors.New("the attach session reported that the container exited, but containerd did not")
		}
	case status := <-statusC:
		// containerd reports the exit before the broker has finished draining
		// the container's stdio. Returning here would run the deferred
		// cancelStream and close the socket with the last frames still queued,
		// losing the tail of the container's output. The broker announces the
		// exit once it is done, which is what ends the stream.
		//
		// The stream's result is checked here too, for the same reason as
		// above. Giving up on the drain is itself a failure: the broker bounds
		// its own flush well below DrainTimeout, so reaching it means output
		// was lost.
		var streamErr error
		select {
		case streamErr = <-streamed:
		case <-time.After(attachmux.DrainTimeout):
			streamErr = errors.New("timed out waiting for the attach session to drain")
		}
		return exitResult(ctx, task, status, streamErr)
	}
}

// exitResult turns a container's exit into the command's result.
//
// sessionErr, when set, is an attach session failure that happened alongside
// the exit. The container's own exit code wins: that is what a caller reads,
// and a broken session at that point only means the tail of the output may be
// missing. A session failure on a container that exited cleanly has nothing
// else to report, so it becomes the error.
func exitResult(ctx context.Context, task containerd.Task, status containerd.ExitStatus, sessionErr error) (containerd.Status, error) {
	cStatus, err := task.Status(ctx)
	if err != nil {
		return cStatus, err
	}
	code, _, err := status.Result()
	if err != nil {
		return cStatus, err
	}
	if code != 0 {
		if sessionErr != nil {
			log.G(ctx).WithError(sessionErr).Warn("the attach session failed, the container output may be incomplete")
		}
		return cStatus, errutil.NewExitCoderErr(int(code))
	}
	if sessionErr != nil {
		return cStatus, fmt.Errorf("the container attach session failed: %w", sessionErr)
	}
	return cStatus, nil
}
