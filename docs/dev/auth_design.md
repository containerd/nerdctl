# [TEMP TITLE] Design document for registry resolution and authentication

<!> IMPORTANT <!>

This document outlines the desired, future behavior of nerdctl.
Current behavior may diverge, or some parts may be missing.
They will be indicated in the document in sections starting with 🤓.

## Preamble

nerdctl supports a set of mechanisms that allows users to control behavior
with regard to registry resolution and authentication.

Generally speaking, and like most tools in the ecosystem, nerdctl strongly encourages
the use of TLS for all communications, as plain http is widely considered insecure
and outright dangerous to use, even in the most restricted and controlled contexts.

Nowadays, setting-up a TLS registry is very simple (thanks to letsencrypt),
and configuring nerdctl to recognize self-signed certificates is also trivial.

Nevertheless, there are still ways to disable TLS certificate validation, or even
force nerdctl to downgrade to plain http communication in certain circumstances.

Note that nerdctl stores and retrieve credentials using docker's credential store implementation,
allowing for some level of interoperability between the docker cli and nerdctl.

Finally, thanks to the [hosts.toml mechanism](https://github.com/containerd/containerd/blob/main/docs/hosts.md),
nerdctl can be instructed to _resolve_ a certain _registry namespace_ to a completely different
_endpoint_, or set of _endpoints_, with fine-grain capabilities.

The interaction between these mechanisms is complex, and if you want to go beyond the simplest
cases (eg: docker cli), you have to understand the implications.

This document purport to extensively cover these.

> 🤓
> - nerdctl currently only support parts of the hosts.toml specification

## Vocabulary

### Registry namespace

A registry namespace is the _host name and port_ that you use
to tag your images with.

In the following example, the _registry namespace_ is `namespace.example:1234`

```bash
nerdctl tag debian namespace.example:1234/my_debian
nerdctl images
```

If there is no specific (`hosts.toml`) configuration on your side, the _registry namespace_
will "resolve" to the following http url: `https://namespace.example:1234/v2/`

The http server at that address will be used when you try to push, or pull, (or login), through a series
of http requests.

Note that omitting a _registry namespace_ from your image name _implies_ that the
_registry namespace_ is `docker.io`.

### Registry host / endpoint

... refers to a fully qualified http url that normally points to an actual, live http server,
able to service the [distribution protocol](https://github.com/opencontainers/distribution-spec).

As mentioned above, you may configure a _registry namespace_ to resolve to different _registry endpoints_,
each with their own set of _allowed capabilities_ (`resolve`, `pull`, and `push`).

What that means is that when you:
```bash
nerdctl pull namespace.example:1234/my_debian
```

... the http endpoint being contacted may very well be `https://somethingelse.example:5678/v2`

### Capabilities

A _registry capability_ refers to a specific registry operation:
- `resolve`: converting a tag (like `latest`) to a digest
- `pull`: retrieving a certain image by digest
- `push`: sending over a locally store image

These distinct capabilities imply different levels of trust.
While it is possible to `pull` an image by digest from an untrusted source,
it is a bad idea to use that same source to `resolve` a tag to a digest,
and even worse to publish an image there.

Granting capabilities to specific _registry endpoints_ is something you control
and decide.

## hosts.toml and registry resolution

In the simplest scenario, as indicated above, without any specific configuration,
the _registry namespace_ `namespace.example:1234` will resolve to the _registry endpoint_
`https://namespace.example:1234/v2/`.

This resolution mechanism can be controlled through the use of `hosts.toml` files.

Said files should be stored under:
- `~/.config/containerd/certs.d/namespace.example:1234/hosts.toml` (for rootless)
- `/etc/containerd/certs.d/namespace.example:1234/hosts.toml` (for rootful)

Note that this mechanism being based on DNS names, ability to control DNS resolution
would obviously allow circumventing this, granted the corresponding registry(-ies) would
service requests on a different hostname.

> 🤓 Note that currently nerdctl only supports resolution for push and pull, but not login.
> This obviously means you currently cannot authenticate against an endpoint.

### hosts.toml file with a "server" directive

The simplest way to configure a different _registry endpoint_ is to use the `server`
section of the `hosts.toml` file:

Effectively, `~/.config/containerd/certs.d/docker.io:443/hosts.toml`
```toml
server = "https://myserver.example:1234"
```

... will make all requests using _namespace_ `docker.io` talk with `myserver.example:1234`.

Note that, in order:
- if you omit the scheme part of the url, `https` is implied
- if you specify any directive applying to the server that implies TLS communication, the scheme will be forced to `https`
- if you omit the port part of the url:
  - port `443` is implied if the scheme is `https`
  - port `80` is implied if the scheme is (explicitly) `http`

Note that if you do omit the server directive in your `hosts.toml`, the default, _implied
host_ for that _namespace_ will be used instead. The _implied host_ for a _namespace_ is decided as:
- take the host (and optional port) of the namespace
- if the port is omitted in the _namespace_, default port 443 is used
- scheme `https` is used, enforcing TLS communication

See section about the `--insecure-registry` flag and `localhost` for exceptions.

### hosts.toml with "hosts" segments

You can further control resolution by adding hosts segments:

```toml
server = "https://myserver.example:1234"

[host."http://another-endpoint.example:4567"]
  capabilities = ["pull", "resolve", "push"]
```

In that case, nerdctl will first try all hosts segments successively with the following algorithm:
- if the host does not specify any capability, it is assumed that is has all capabilities
- if the host has a capability that matches the requested operation, try it
  - if the operation is successful with that host, we are done
  - if the operation was unsuccesful, continue to the next host
- if the host does not have the capability to match the requested operation, continue to the next host

Once all configured hosts have been exhausted unsuccessfully, nerdctl will try the `server`
(explicit or implied).

Note that hosts directives use the same heuristic as server with regard to scheme and port.

### Non-compliant hosts

Hosts that do implement the protocol correctly should serve under the `/v2/` root path.

To configure a non-compliant host, you may pass along `override_path = true` as a property,
and specify the full url you expect in the host segment.

### TLS configuration, custom headers, etc...

Both server and hosts segments can specify custom TLS configuration, like a custom CA,
client certificates, and the ability to skip verification of TLS certificates, along
with the ability to pass additional http headers.

TL;DR:
```toml
  ca = "/etc/certs/myca.pem"
  skip_verify = false
  client = [["/etc/certs/client.cert", "/etc/certs/client.key"],["/etc/certs/client.pem", ""]]
  [header]
    x-custom = "my custom header"
```

Refer to the `hosts.toml` dedicated documentation for details.

## HTTP requests

Requests sent to a configured `server` or `host` will add a query parameter to the urls.
For example:

```bash
http://myserver.example/v2/library/debian/manifests/latest?ns=docker.io
```

This allows registry servers to understand for what namespace they are serving
resources, and possibly perform additional operations.

Obviously, nothing prevents a registry server to be used both as a default server
for a namespace, and also as an endpoint for another.

## What happens with localhost?

If localhost is used as a _registry namespace_ without any specific configuration,
it is by default treated as if the following had been set in its toml file:

`~/.config/containerd/certs.d/localhost:443/hosts.toml`
```toml
server = "http://localhost:80"

[host."https://localhost:443"]
  skip_verify = true
```

Specifying a port (`localhost:1234`) will not change the overall behavior.
It will be equivalent to setting the following file:

`~/.config/containerd/certs.d/localhost:1234/hosts.toml`
```toml
[host."https://localhost:1234"]
  skip_verify = true
[host."http://localhost:1234"]
```

This behavior is historical (and subject to change by docker as well), and can be disabled
for nerdctl by passing an explicit `--insecure-registry=false`, in which case `localhost` will be treated
as any other namespace.

All of the above solely applies when `localhost` is used as an un-configured namespace.

> 🤓 currently, nerdctl will treat `--insecure-registry=false` the same way as if the flag was not passed.

## What does `nerdctl --insecure-registry` do?

This is a custom flag supported only by nerdctl (docker does not support it).

Using it is discouraged, as its design is inconsistent with the `hosts.toml` mechanism
which should be used instead.

The flag only applies when used against a _registry namespace_ with **no** explicit hosts.toml
configuration.
In that scenario, when `--insecure-registry=true` is specified, it will behave as if the
following hosts.toml had been configured.

For namespace `mynamespace.example` (no port):

`~/.config/containerd/certs.d/mynamespace.example:443/hosts.toml`
```toml
server = "http://mynamespace.example:80"
[host."https://mynamespace.example:443"]
  skip_verify = true
```

For namespace `mynamespace.example:1234`:

`~/.config/containerd/certs.d/mynamespace.example:1234/hosts.toml`
```toml
server = "http://mynamespace.example:1234"
[host."https://mynamespace.example:1234"]
  skip_verify = true
```

For namespace `mynamespace.example:443`:

`~/.config/containerd/certs.d/mynamespace.example:443/hosts.toml`
```toml
server = "http://mynamespace.example:443"
[host."https://mynamespace.example:443"]
  skip_verify = true
```

For namespace `mynamespace.example:80`:

`~/.config/containerd/certs.d/mynamespace.example:80/hosts.toml`
```toml
server = "http://mynamespace.example:80"
[host."https://mynamespace.example:80"]
  skip_verify = true
```

The effect of `--insecure-registry=false` is generally a no-op, except in the case of
localhost as described above.

Note that using `--insecure-registry=true` on a namespace that DO have an explicit `hosts.toml`
configuration is a no-op as well.

> 🤓 currently, it seems like `insecure-registry` will be applied to endpoints as well (though login is not working).

## Authentication

In its simple form, `nerdctl login` will behave exactly
the same way as docker (which does not support `hosts.toml`).

For example:
```nerdctl login namespace.example```

Will resolve to the implied _registry endpoint_ `https://namespace.example:443/`
and authenticate there either prompting for credentials, or, if they exist,
retrieving credentials from the docker store.

The `--insecure-registry` flag will work in that case with the same semantics as
outlined above.

Now, when `server` and `hosts` configuration are involved, the behavior is different.

If there are `host` directives:

1. and there is no `server` directive, or if the `server` directive matches the scheme, domain and port
of the requested _registry namespace_ implied server, `nerdctl login` will function as above,
but will additionally notify the user that additional endpoints exist for that namespace,
and instruct the user to log in to these endpoints additionally if they need to.
2. if on the other hand there is a `server` directive that does NOT match the namespace host,
`nerdctl login` will decline to log in, and instruct the user to use the endpoint login syntax instead

To log in into a specific _endpoint_ for a _registry namespace_, you should use the
additional login flag `--endpoint`.

For example:
```bash
nerdctl login namespace.example --endpoint myserver.example
```

Will proceed with the following steps:
- check that there is indeed a `myserver.example` endpoint configured in the hosts.toml for `namespace.example`
- if there is one, try to authenticate against `https://myserver.example:443/v2/?ns=https://namespace.example:443`

Note that:
- implied scheme and port resolution follow the same rules outlined above,
both for the namespace and the endpoint
- the flag `--insecure-registry` is a no-op

> 🤓 currently, nerdctl does not allow the user to login when there is an explicit hosts.toml configuration.
> Put otherwise, nerdctl only allows the user to login to raw namespaces.
> This proposal, especially the --endpoint flag will allow login to configured namespaces and endpoints.

## Credentials storage

As outlined, credentials are stored using docker facilities.

This is usually stored inside the file `$DOCKER_CONFIG/config.json`,
and credentials are keyed per-namespace host (domain+port), except for
the docker hub registry which uses a fully qualified URL.

Since docker does not support `hosts.toml` and since _endpoints_ are not
the same thing as an implied registry host for a namespace, we store
_endpoint_ credentials using a different schema.

Docker will not recognize this schema, hence will not wrongly send these
credentials when trying to log in into a known _endpoint_ as a registry.

The schema is: `nerdctl-experimental://namespace.example:123/?endpoint=myserver.example:456`

As clearly shown above, this is currently experimental, and is subject to change
in the future.
There is no guarantees that credentials stored that way will be able to be retrieved
by future nerdctl versions.

> 🤓 as outlined above, nerdctl-experimental is a new proposed behavior to support login
> with configured namespaces.