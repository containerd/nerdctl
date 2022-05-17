# Interactive debugging of Dockerfile (Experimental)

nerdctl supports interactive debugging of Dockerfile as `nerdctl builder debug`.

```
$ nerdctl builder debug /path/to/context
```

This feature leverages [buildg](https://github.com/ktock/buildg), interactive debugger of Dockerfile.
For command reference, please refer to the [Command reference doc in buildg repo](https://github.com/ktock/buildg#command-reference).

:warning: This command currently doesn't use the host's `buildkitd` daemon but uses the patched version of BuildKit provided by buildg. This should be fixed to use the host's `buildkitd` in the future.

## Example

Example Dockerfile:

```Dockerfile
FROM busybox AS build1
RUN echo a > /a
RUN echo b > /b
RUN echo c > /c
```

Example debugging:

```console
$ nerdctl builder debug --image=ubuntu:22.04 /tmp/ctx/
WARN[2022-05-17T10:15:48Z] using host network as the default#1 [internal] load .dockerignore
#1 transferring context: 2B done
#1 DONE 0.1s

#2 [internal] load build definition from Dockerfile
#2 transferring dockerfile: 108B done
#2 DONE 0.1s

#3 [internal] load metadata for docker.io/library/busybox:latest
INFO[2022-05-17T10:15:51Z] debug session started. type "help" for command reference.
Filename: "Dockerfile"
 =>   1| FROM busybox AS build1
      2| RUN echo a > /a
      3| RUN echo b > /b
      4| RUN echo c > /c
(buildg) break 3
(buildg) breakpoints
[0]: line: Dockerfile:3
[on-fail]: breaks on fail
(buildg) continue
#3 DONE 3.1s

#4 [1/4] FROM docker.io/library/busybox@sha256:d2b53584f580310186df7a2055ce3ff83cc0df6caacf1e3489bff8cf5d0af5d8
#4 resolve docker.io/library/busybox@sha256:d2b53584f580310186df7a2055ce3ff83cc0df6caacf1e3489bff8cf5d0af5d8 0.0s done
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0B / 772.81kB 0.2s
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0B / 772.81kB 5.3s
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0B / 772.81kB 10.4s
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 772.81kB / 772.81kB 11.4s done
#4 extracting sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0.1s done
#4 DONE 20.2s

#5 [2/4] RUN echo a > /a
#5 DONE 0.1s
Breakpoint[0]: reached line: Dockerfile:3
Filename: "Dockerfile"
      1| FROM busybox AS build1
      2| RUN echo a > /a
*=>   3| RUN echo b > /b
      4| RUN echo c > /c
(buildg) exec --image sh
# ls /debugroot/
a  b  bin  dev	etc  home  proc  root  tmp  usr  var
# cat /debugroot/a /debugroot/b
a
b
#
(buildg) quit
```
