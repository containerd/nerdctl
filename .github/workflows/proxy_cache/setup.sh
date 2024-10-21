#!/usr/bin/env bash
# shellcheck disable=SC2034,SC2015
set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root

# Cleanup for repeat local development
docker rmi -f caddy
docker rm -f proxy
# Build the caddy image
docker build -t caddy -f "$root"/Dockerfile "$root"
# Run it, exposing 80 and 443 (+2024 for the admin / trust)
docker run -d --restart always --name proxy -p 80:80 -p 443:443 -p 2024:2024 caddy
# Copy caddy here and trust the generated certificate
docker cp proxy:/go/caddy .
./caddy trust --address localhost:2024
# Point docker registry to our proxy
echo "127.0.0.1 registry-1.docker.io" | sudo tee -a /etc/hosts >/dev/null
echo "127.0.0.1 ports.ubuntu.com" | sudo tee -a /etc/hosts >/dev/null
echo "127.0.0.1 deb.debian.org" | sudo tee -a /etc/hosts >/dev/null
# Restart docker to take into account the newly trusted certificate
sudo systemctl restart docker
