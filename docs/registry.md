# registry authentication

nerdctl uses `${DOCKER_CONFIG}/config.json` for the authentication with image registries.

`$DOCKER_CONFIG` defaults to `$HOME/.docker`.

## Using insecure registry

If you face `http: server gave HTTP response to HTTPS client` and you cannot configure TLS for the registry, try `--insecure-registry` flag:

e.g.,
```console
$ nerdctl --insecure-registry run --rm 192.168.12.34:5000/foo
```

## Specifying certificates


| :zap: Requirement | nerdctl >= 0.16 |
|-------------------|-----------------|


Create `~/.config/containerd/certs.d/<HOST:PORT>/hosts.toml` (or `/etc/containerd/certs.d/...` for rootful) to specify `ca` certificates.

```toml
# An example of ~/.config/containerd/certs.d/192.168.12.34:5000/hosts.toml
# (The path is "/etc/containerd/certs.d/192.168.12.34:5000/hosts.toml" for rootful)

server = "https://192.168.12.34:5000"
[host."https://192.168.12.34:5000"]
  ca = "/path/to/ca.crt"
```

See https://github.com/containerd/containerd/blob/main/docs/hosts.md for the syntax of `hosts.toml` .

Docker-style directories are also supported.
The path is `~/.config/docker/certs.d` for rootless, `/etc/docker/certs.d` for rootful.

## Accessing 127.0.0.1 from rootless nerdctl

Currently, rootless nerdctl cannot pull images from 127.0.0.1, because
the pull operation occurs in RootlessKit's network namespace.

See https://github.com/containerd/nerdctl/issues/86 for the discussion about workarounds.

- - -

# Using managed registry services
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->


- [Amazon Elastic Container Registry (ECR)](#amazon-elastic-container-registry-ecr)
  - [Logging in](#logging-in)
  - [Creating a repo](#creating-a-repo)
  - [Pushing an image](#pushing-an-image)
- [Azure Container Registry (ACR)](#azure-container-registry-acr)
  - [Creating a registry](#creating-a-registry)
  - [Logging in](#logging-in-1)
  - [Creating a repo](#creating-a-repo-1)
  - [Pushing an image](#pushing-an-image-1)
- [Docker Hub](#docker-hub)
  - [Logging in](#logging-in-2)
  - [Creating a repo](#creating-a-repo-2)
  - [Pushing an image](#pushing-an-image-2)
- [GitHub Container Registry (GHCR)](#github-container-registry-ghcr)
  - [Logging in](#logging-in-3)
  - [Creating a repo](#creating-a-repo-3)
  - [Pushing an image](#pushing-an-image-3)
- [GitLab Container Registry](#gitlab-container-registry)
  - [Logging in](#logging-in-4)
  - [Creating a repo](#creating-a-repo-4)
  - [Pushing an image](#pushing-an-image-4)
- [Google Artifact Registry (pkg.dev)](#google-artifact-registry-pkgdev)
  - [Logging in](#logging-in-5)
  - [Creating a repo](#creating-a-repo-5)
  - [Pushing an image](#pushing-an-image-5)
- [Google Container Registry (GCR) [DEPRECATED]](#google-container-registry-gcr-deprecated)
  - [Logging in](#logging-in-6)
  - [Creating a repo](#creating-a-repo-6)
  - [Pushing an image](#pushing-an-image-6)
- [JFrog Artifactory (Cloud/On-Prem)](#jfrog-artifactory-cloudon-prem)
  - [Logging in](#logging-in-7)
  - [Creating a repo](#creating-a-repo-7)
  - [Pushing an image](#pushing-an-image-7)
- [Quay.io](#quayio)
  - [Logging in](#logging-in-8)
  - [Creating a repo](#creating-a-repo-8)
  - [Pushing an image](#pushing-an-image-8)

<!-- END doctoc generated TOC please keep comment here to allow auto update -->

## Amazon Elastic Container Registry (ECR)

See also https://aws.amazon.com/ecr

### Logging in

```console
$ aws ecr get-login-password --region <REGION> | nerdctl login --username AWS --password-stdin <AWS_ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com
Login Succeeded
```

<details>
<summary>Alternative method: <code>docker-credential-ecr-login</code></summary>

This methods is more secure but needs an external dependency.

<p>

Install `docker-credential-ecr-login` from https://github.com/awslabs/amazon-ecr-credential-helper , and create the following files:

`~/.docker/config.json`:
```json
{
  "credHelpers": {
    "public.ecr.aws": "ecr-login",
    "<AWS_ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com": "ecr-login"
  }
}
```

`~/.aws/credentials`:
```
[default]
aws_access_key_id = ...
aws_secret_access_key = ...
```

> **Note**: If you are running nerdctl inside a VM (including Lima, Colima, Rancher Desktop, and WSL2), `docker-credential-ecr-login` has to be installed inside the guest, not the host.
> Same applies to the path of `~/.docker/config.json` and `~/.aws/credentials`, too.

</p>
</details>

### Creating a repo

You have to create a repository via https://console.aws.amazon.com/ecr/home/ .

### Pushing an image

```console
$ nerdctl tag hello-world <AWS_ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/<REPO>
$ nerdctl push <AWS_ACCOUNT_ID>.dkr.ecr.<REGION>.amazonaws.com/<REPO>
```

The pushed image appears in the repository you manually created in the previous step.

## Azure Container Registry (ACR)
See also https://azure.microsoft.com/en-us/services/container-registry/#overview

### Creating a registry

You have to create a "Container registry" resource manually via [the Azure portal](https://portal.azure.com/).

### Logging in
```console
$ nerdctl login -u <USERNAME> <REGISTRY>.azurecr.io
Enter Password: ********[Enter]

Login Succeeded
```

The login credentials can be found as "Access keys" in [the Azure portal](https://portal.azure.com/).
See also https://docs.microsoft.com/en-us/azure/container-registry/container-registry-authentication .

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
You do not need to create a repo explicitly.

### Pushing an image

```console
$ nerdctl tag hello-world <REGISTRY>.azurecr.io/hello-world
$ nerdctl push <REGISTRY>.azurecr.io/hello-world
```

The pushed image appears in [the Azure portal](https://portal.azure.com/).
Private as default.

## Docker Hub
See also https://hub.docker.com/

### Logging in
```console
$ nerdctl login -u <USERNAME>
Enter Password: ********[Enter]

Login Succeeded
```

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
You do not need to create a repo explicitly, for public images.

To create a private repo, see https://hub.docker.com/repositories .

### Pushing an image

```console
$ nerdctl tag hello-world <USERNAME>/hello-world
$ nerdctl push <USERNAME>/hello-world
```

The pushed image appears in https://hub.docker.com/repositories .
**Public** by default.

## GitHub Container Registry (GHCR)
See also https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry

### Logging in

```console
$ nerdctl login ghcr.io -u <USERNAME>
Enter Password: ********[Enter]

Login Succeeded
```

The `<USERNAME>` is your GitHub username but in lower characters.

The "Password" here is a [GitHub Personal access token](https://github.com/settings/tokens), with `read:packages` and `write:packages` scopes.

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
You do not need to create a repo explicitly.

### Pushing an image

```console
$ nerdctl tag hello-world ghcr.io/<USERNAME>/hello-world
$ nerdctl push ghcr.io/<USERNAME>/hello-world
```

The pushed image appears in the "Packages" tab of your GitHub profile.
Private as default.

## GitLab Container Registry
See also https://docs.gitlab.com/ee/user/packages/container_registry/

### Logging in

```console
$ nerdctl login registry.gitlab.com -u <USERNAME>
Enter Password: ********[Enter]

Login Succeeded
```

The `<USERNAME>` is your GitLab username.

The "Password" here is either a [GitLab Personal access token](https://docs.gitlab.com/ee/user/profile/personal_access_tokens.html) or a [GitLab Deploy token](https://docs.gitlab.com/ee/user/project/deploy_tokens/index.html). Both options require minimum scope of `read_registry` for pull access and both `write_registry` and `read_registry` scopes for push access.

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
Container registries in GitLab are created at the project level. A project in GitLab must exist first before you begin working with its container registry.

### Pushing an image

In this example we have created a GitLab project named `myproject`.

```console
$ nerdctl tag hello-world registry.gitlab.com/<USERNAME>/myproject/hello-world:latest
$ nerdctl push registry.gitlab.com/<USERNAME>/myproject/hello-world:latest
```

The pushed image appears under the "Packages & Registries -> Container Registry" tab of your project on GitLab.

## Google Artifact Registry (pkg.dev)
See also https://cloud.google.com/artifact-registry/docs/docker/quickstart

### Logging in

Create a [GCP Service Account](https://cloud.google.com/iam/docs/creating-managing-service-accounts#creating), grant
`Artifact Registry Reader` and `Artifact Registry Writer` roles, and download the key as a JSON file.

Then run the following command:

```console
$ cat <GCP_SERVICE_ACCOUNT_KEY_JSON> | nerdctl login -u _json_key --password-stdin https://<REGION>-docker.pkg.dev
WARNING! Your password will be stored unencrypted in /home/<USERNAME>/.docker/config.json.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store

Login Succeeded
```

See also https://cloud.google.com/artifact-registry/docs/docker/authentication


<details>
<summary>Alternative method: <code>docker-credential-gcloud</code> (<code>gcloud auth configure-docker</code>)</summary>

This methods is more secure but needs an external dependency.

<p>

Run `gcloud auth configure-docker <REGION>-docker.pkg.dev`, e.g.,

```console
$ gcloud auth configure-docker asia-northeast1-docker.pkg.dev
Adding credentials for: asia-northeast1-docker.pkg.dev
After update, the following will be written to your Docker config file located at [/home/<USERNAME>/.docker/config.json]:
 {
  "credHelpers": {
    "asia-northeast1-docker.pkg.dev": "gcloud"
  }
}

Do you want to continue (Y/n)?  y

Docker configuration file updated.
```

Google Cloud SDK (`gcloud`, `docker-credential-gcloud`) has to be installed, see https://cloud.google.com/sdk/docs/quickstart .

> **Note**: If you are running nerdctl inside a VM (including Lima, Colima, Rancher Desktop, and WSL2), the Google Cloud SDK has to be installed inside the guest, not the host.

</p>
</details>

### Creating a repo

You have to create a repository via https://console.cloud.google.com/artifacts .
Choose "Docker" as the repository format.

### Pushing an image

```console
$ nerdctl tag hello-world <REGION>-docker.pkg.dev/<GCP_PROJECT_ID>/<REPO>/hello-world
$ nerdctl push <REGION>-docker.pkg.dev/<GCP_PROJECT_ID>/<REPO>/hello-world
```

The pushed image appears in the repository you manually created in the previous step.

## Google Container Registry (GCR) [DEPRECATED]
See also https://cloud.google.com/container-registry/docs/advanced-authentication

### Logging in

Create a [GCP Service Account](https://cloud.google.com/iam/docs/creating-managing-service-accounts#creating), grant
`Storage Object Admin` role, and download the key as a JSON file.

Then run the following command:

```console
$ cat <GCP_SERVICE_ACCOUNT_KEY_JSON> | nerdctl login -u _json_key --password-stdin https://asia.gcr.io
WARNING! Your password will be stored unencrypted in /home/<USERNAME>/.docker/config.json.
Configure a credential helper to remove this warning. See
https://docs.docker.com/engine/reference/commandline/login/#credentials-store

Login Succeeded
```

See also https://cloud.google.com/container-registry/docs/advanced-authentication

<details>
<summary>Alternative method: <code>docker-credential-gcloud</code> (<code>gcloud auth configure-docker</code>)</summary>

This methods is more secure but needs an external dependency.

<p>

```console
$ gcloud auth configure-docker
Adding credentials for all GCR repositories.
WARNING: A long list of credential helpers may cause delays running 'docker build'. We recommend passing the registry name to configure only the registry you are using.
After update, the following will be written to your Docker config file located at [/home/<USERNAME>/.docker/config.json]:
 {
  "credHelpers": {
    "gcr.io": "gcloud",
    "us.gcr.io": "gcloud",
    "eu.gcr.io": "gcloud",
    "asia.gcr.io": "gcloud",
    "staging-k8s.gcr.io": "gcloud",
    "marketplace.gcr.io": "gcloud"
  }
}

Do you want to continue (Y/n)?  y

Docker configuration file updated.
```

Google Cloud SDK (`gcloud`, `docker-credential-gcloud`) has to be installed, see https://cloud.google.com/sdk/docs/quickstart .

> **Note**: If you are running nerdctl inside a VM (including Lima, Colima, Rancher Desktop, and WSL2), the Google Cloud SDK has to be installed inside the guest, not the host.

</p>
</details>

### Creating a repo
You do not need to create a repo explicitly.

### Pushing an image

```console
$ nerdctl tag hello-world asia.gcr.io/<GCP_PROJECT_ID>/hello-world
$ nerdctl push asia.gcr.io/<GCP_PROJECT_ID>/hello-world
```

The pushed image appears in https://console.cloud.google.com/gcr/ .
Private by default.

## JFrog Artifactory (Cloud/On-Prem)
See also https://www.jfrog.com/confluence/display/JFROG/Getting+Started+with+Artifactory+as+a+Docker+Registry

### Logging in
```console
$ nerdctl login <SERVER_NAME>.jfrog.io -u <USERNAME>
Enter Password: ********[Enter]

Login Succeeded
```

Login using the default username: admin, and password: password for the on-prem installation, or the credentials provided to you by email for the cloud installation.
JFrog Platform is integrated with OAuth allowing you to delegate authentication requests to external providers (the provider types supported are Google, OpenID Connect, GitHub Enterprise, and Cloud Foundry UAA)

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
1. Add local Docker repository
   1. Add a new Local Repository with the Docker package type via `https://<server-name>.jfrog.io/ui/admin/repositories/local/new`.
2. Add virtual Docker repository
   1. Add a new virtual repository with the Docker package type via `https://<server-name>.jfrog.io/ui/admin/repositories/virtual/new`.
   2. Add the local docker repository you created in Steps 1 (move it from Available Repositories to Selected Repositories using the arrow buttons).
   3. Set local repository as a default local deployment repository.

### Pushing an image
```console
$ nerdctl tag hello-world <SERVER_NAME>.jfrog.io/<VIRTUAL_REPO_NAME>/hello-world
$ nerdctl push <SERVER_NAME>.jfrog.io/<VIRTUAL_REPO_NAME>/hello-world
```

The `SERVER_NAME` is the first part of the URL given to you for your environment: `https://<SERVER_NAME>.jfrog.io`

The `VIRTUAL_REPO_NAME` is the name “docker” that you assigned to your virtual repository in 2.i .

The pushed image appears in `https://<SERVER_NAME>.jfrog.io/ui/repos/tree/General/<VIRTUAL_REPO_NAME>` .
Private by default.

## Quay.io
See also https://docs.quay.io/solution/getting-started.html

### Logging in

```console
$ nerdctl login quay.io -u <USERNAME>
Enter Password: ********[Enter]

Login Succeeded
```

> **Note**: nerdctl prior to v0.16.1 had a bug that required pressing the Enter key twice.

### Creating a repo
You do not need to create a repo explicitly.

### Pushing an image

```console
$ nerdctl tag hello-world quay.io/<USERNAME>/hello-world
$ nerdctl push quay.io/<USERNAME>/hello-world
```

The pushed image appears in https://quay.io/repository/ .
Private as default.
