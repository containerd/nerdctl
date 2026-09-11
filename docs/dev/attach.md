# How attach works

A container's stdio FIFOs cannot be shared. A FIFO has one queue, so two readers
on the stdout FIFO each receive a random subset of the container's output: one
terminal appears to freeze while the other prints characters it never asked for.
That was https://github.com/containerd/nerdctl/issues/4374.

Docker does not have this problem because `dockerd` owns the container's stdio
and fans it out. nerdctl is daemonless, so a different process has to own it.

## The broker

The owner is the internal logging process, the one nerdctl runs as
`_NERDCTL_INTERNAL_LOGGING` (`pkg/logging`). containerd spawns it from the
container's log URI, so it exists for the container's whole lifetime and comes
back on its own when the containerd restart monitor recreates a task.

It gains three things (`pkg/logging/broker_unix.go`):

- a unix socket at a path derived from the data store, the namespace and the
  container ID. Nothing is recorded on disk: clients derive the same path with
  the same function, and ask containerd whether the task's stdio belongs to a
  logging process at all
- a fan-out of the container's raw output, tapped before the log driver's line
  splitting so that partial lines and terminal escape sequences survive
- the write end of the container's stdin FIFO, which every session's input is
  merged into

`nerdctl run -it`, `nerdctl start -a` and `nerdctl attach` are all clients
(`pkg/attachmux`).

## Invariants

- **The container never blocks on a session.** Each session has a bounded queue;
  one that falls behind is disconnected. The goroutine reading the container's
  stdio never waits on a consumer.
- **With no sessions attached the broker keeps draining.** Detaching therefore
  leaves the container running normally.
- **There is no replay buffer.** `run` and `start -a` connect before the task
  starts, so no output is produced with nobody attached. Attaching to a running
  container shows no history, as with docker.

## Stdin

`cioutil.StdinFIFOPath` pins a container's stdin to
`<dataStore>/containers/<ns>/<id>/stdin.fifo`. containerd generates a fresh FIFO
directory per task, so those paths change across restarts and another process
cannot derive them.

For a terminal container the containerd shim copies the stdin FIFO into the pty
regardless of the stdout URI scheme
(`cmd/containerd-shim-runc-v2/runc/platform.go`, `CopyConsole`), which is what
lets a container keep stdin while its output goes to the logging process. For a
non-terminal container the `binary` scheme sets `pio.copy = false` and
`binaryIO.Stdin()` returns nil (`cmd/containerd-shim-runc-v2/process/io.go`,
`createIO`), so stdin is not wired at all. That is why multi-session stdin is
currently limited to terminal containers.

## After a restart-policy restart, sessions are output only

containerd's restart monitor recreates a task from the stored log URI. For a
terminal container it does that through `cio.TerminalLogURI`
(`plugins/restart/change.go`), whose config is `{Terminal, Stdout, Stderr}` and
carries no `Stdin` (`pkg/cio/io_unix.go`). The shim therefore never opens the
read end of the stable stdin FIFO, and the broker serves output only until the
container is started by nerdctl again.

The FIFO file itself is still there from the previous run, which is why
`openStdinWriter` decides by opening it rather than by looking for it: a FIFO
opened `O_WRONLY|O_NONBLOCK` fails with `ENXIO` for as long as nothing holds the
read end. `github.com/containerd/fifo` deliberately hides that - given
`O_NONBLOCK` it strips the flag and opens in a goroutine - so it cannot be used
here.

The broker itself does come back, because it is spawned from the log URI, so
output fan-out and logging keep working. Only input is lost. This is not a
regression: such a container has no stdin today either. Closing it needs the
restart monitor to preserve the full IO config, which is an upstream change.

## Backing it out

`disable_attach_broker` (`--disable-attach-broker`, `NERDCTL_DISABLE_ATTACH_BROKER`,
`nerdctl.toml`) leaves the data store empty at task creation, which is what a
container with a foreign log driver already looks like, so every branch above
takes the legacy path without any new code. It only affects containers created
while it is set: a running container's stdio is already bound.

## Falling back

`nerdctl attach` asks containerd how the task's stdio is wired, through the
`cio.Attach` callback that `container.Task` hands the task's recorded `Stdin`,
`Stdout` and `Stderr`. A plain path means FIFOs, and it attaches to them
directly the way it always did, one session at a time: that is what a foreground
`run -it` looks like when the socket was unavailable, and what an older nerdctl
left behind. A URI means the output went to a process spawned from the log URI,
and the socket is the only way in.

Not every URI-owned container has a reachable socket. A detached container has
had URI stdio since long before this change, on every platform, so one created by
an older nerdctl or running where the transport is not implemented reports that
its stdio is owned by a process nerdctl cannot reach. `nerdctl attach` fails
there today too, inside `fifo.OpenFifo` on a path spelled `binary://...`; only
the message is new.

The classification and the IO construction happen in the same `container.Task`
call. Splitting them would leave a window in which the restart monitor replaces
a FIFO task with a URI one between the two `TaskService.Get` round trips.

Nothing about this is cached in nerdctl's own state. It cannot be: containerd's
restart monitor recreates a task from the stored log URI without nerdctl running
at all, so a container that had FIFO stdio can come back with URI stdio between
two nerdctl invocations.

There is no falling back the other way. A task built for the broker has
`binary://` URIs where `cio.NewAttach` expects FIFO paths, so a socket that
cannot be dialled is reported as an error rather than retried on a path that
cannot work. A container whose URI points at somebody else's logging binary
therefore cannot be attached to at all.

`nerdctl run -it` decides which kind of task to create before the task exists.
`attachmux.Probe` binds a throwaway socket under the data store up front so the
common failures pick the legacy path while that is still possible, as does a
stable stdin FIFO that could not be created for a `-i` session. If the socket is
unreachable afterwards the task is deleted before it starts, so the user gets a
clean error rather than a container they cannot see.