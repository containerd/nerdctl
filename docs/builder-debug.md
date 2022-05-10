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
WARN[2022-05-10T12:49:43Z] using host network as the default
#1 [internal] load .dockerignore
#1 transferring context: 2B done
#1 DONE 0.1s

#2 [internal] load build definition from Dockerfile
#2 transferring dockerfile: 108B done
#2 DONE 0.1s

#3 [internal] load metadata for docker.io/library/busybox:latest
#3 DONE 3.0s

#4 [1/4] FROM docker.io/library/busybox@sha256:d2b53584f580310186df7a2055ce3ff83cc0df6caacf1e3489bff8cf5d0af5d8
#4 resolve docker.io/library/busybox@sha256:d2b53584f580310186df7a2055ce3ff83cc0df6caacf1e3489bff8cf5d0af5d8 0.0s done
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0B / 772.81kB 0.2s
Filename: "Dockerfile"
      1| FROM busybox AS build1
 =>   2| RUN echo a > /a
      3| RUN echo b > /b
      4| RUN echo c > /c
>>> break 3
>>> breakpoints
[0]: line: Dockerfile:3
[on-fail]: breaks on fail
>>> continue
#4 sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 772.81kB / 772.81kB 0.3s done
#4 extracting sha256:50e8d59317eb665383b2ef4d9434aeaa394dcd6f54b96bb7810fdde583e9c2d1 0.0s done
#4 DONE 0.4s

#5 [2/4] RUN echo a > /a
#5 DONE 15.1s
Breakpoint: line: Dockerfile:3: reached
Filename: "Dockerfile"
      1| FROM busybox AS build1
      2| RUN echo a > /a
*=>   3| RUN echo b > /b
      4| RUN echo c > /c
>>> exec --image sh
# cat /etc/os-release
PRETTY_NAME="Ubuntu 22.04 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
VERSION="22.04 LTS (Jammy Jellyfish)"
VERSION_CODENAME=jammy
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"
SUPPORT_URL="https://help.ubuntu.com/"
BUG_REPORT_URL="https://bugs.launchpad.net/ubuntu/"
PRIVACY_POLICY_URL="https://www.ubuntu.com/legal/terms-and-policies/privacy-policy"
UBUNTU_CODENAME=jammy
# ls /debugroot/
a  b  bin  dev	etc  home  proc  root  tmp  usr  var
# cat /debugroot/a /debugroot/b
a
b
#
>>> quit
```
