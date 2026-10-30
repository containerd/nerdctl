# Reproducing JSON log write failures

The JSON logger must keep draining stdout and stderr when its file is full, and
resume writing later records after space is available. Losing records while the
sink rejects writes is expected; permanently blocking output or permanently
losing future logs is not.

## Run the reproducer

Use a disposable Linux host with root access, containerd, Bash, and standard
`mount`, `umount`, `dd`, and `grep` commands. The script pulls Alpine 3.22 from
Docker Hub. No application, service credentials, or existing container is needed.

```sh
sudo bash docs/testing/repro-json-log-enospc.sh /path/to/nerdctl
```

The script creates a unique namespace and container, mounts a private **8 MiB
tmpfs**, fills only that mount, and produces 30,000 lines on each output stream.
A control file outside the full mount records application progress. After three
seconds, it removes the filler and checks that the workload finishes and both
`RECOVERED_STDOUT` and `RECOVERED_STDERR` appear in the log. It removes its
container, unmounts the filesystem, and deletes its temporary files on exit.
Pulled image content may remain in containerd's cache.

Log rotation is bounded with `max-size=1m,max-file=2` so the workload does not
immediately fill the recovered filesystem again. The script uses `overlayfs` by
default. Set `SNAPSHOTTER=native` if that is the configured snapshotter. With
containerd's transfer service, that snapshotter must also be enabled in its
`unpack_config`. `CONTAINERD_ADDRESS` selects a nondefault daemon socket.

An unpatched run exits 1 and reports blocked output or missing recovery markers.
A successful fixed run exits 0:

```text
Injected ENOSPC on private 8 MiB log filesystem
Progress before freeing space: <workload-dependent count>
PASS: container completed 30000 writes per stream and both logs recovered
```

Use a fresh container for each binary: replacing nerdctl does not replace an
already-running logging subprocess. The script does this automatically.

## What fails

In the asynchronous path (`jsonfile.Encode`, used by nerdctl 2.2.2), a failed
write terminates a consumer. The producer channel then fills, followed by the
container's stdout/stderr pipe. Freeing space does not restart that consumer.

Current source also has `SyncEncoder`, used for synchronous JSON writes. Go's
`json.Encoder` retains its first writer error. Continuing to call the same
encoder therefore cannot recover even when the underlying writer can accept
bytes again. The synchronous path needs recovery too; it must not be described
as having the same consumer lifecycle as 2.2.2.

The fix keeps the asynchronous consumers running and retries the underlying
writer on later entries. After an error, a newline terminates any partial
record and a fresh JSON encoder writes subsequent entries. Failed entries are
dropped. A pre-existing partial record is not repaired or removed: reading
across that malformed record can still fail until rotation removes it. This
change does not make indefinitely blocking storage writes nonblocking.

## Fast regression checks

```sh
go test -race -count=20 ./pkg/logging/jsonfile
go test ./pkg/logging/...
```

The tests cover partial writes followed by recovery, synchronous recovery, and
continued consumption of both streams while the writer stays full. They fail
against the unchanged implementation and pass with the fix. The shell
reproducer complements those tests with an actual ENOSPC filesystem error and
real container stdout/stderr.
