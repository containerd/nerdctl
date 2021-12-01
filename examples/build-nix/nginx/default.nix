# An example of reproducible image building using Nix
#
# Usage: nerdctl build-nix && nerdctl run -d -p 8080:80 nginx:nix
#
# Requires nerdctl >= 0.15.0

# Hints:
# - https://nix.dev/tutorials/building-and-running-docker-images
# - https://nixos.org/manual/nixpkgs/stable/#ssec-pkgs-dockerTools-buildImage
# - https://github.com/NixOS/nixpkgs/blob/master/pkgs/build-support/docker/examples.nix
# - https://nix.dev/tutorials/towards-reproducibility-pinning-nixpkgs

{ sources ? import ./nix/sources.nix, pkgs ? import sources.nixpkgs {} }:
pkgs.dockerTools.buildImage{
  name = "nginx";
  tag = "nix";
  contents = [
    # fakeNss creates /etc/passwd and /etc/group (https://github.com/NixOS/nixpkgs/blob/e548124f/pkgs/build-support/docker/default.nix#L741-L763)
    pkgs.dockerTools.fakeNss
    pkgs.bash
    pkgs.coreutils
    pkgs.nginx
    (pkgs.writeTextDir "${pkgs.nginx}/html/index.html" ''
    <html><body>hello nix</body></html>
    '')
  ];
  extraCommands = "
    grep -q ^nogroup etc/group || echo nogroup:x:65534: >>etc/group
    mkdir -p var/log/nginx var/cache/nginx/client_body
  ";
  config = {
    Cmd = [ "nginx" "-g" "daemon off; error_log /dev/stderr debug;"];
    ExposedPorts = {
      "80/tcp" = {};
    };
  };
}
