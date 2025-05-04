#!/usr/bin/env bash

#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root

# Cli to uses - nerdctl or docker
readonly cli="${CLI:-nerdctl}"
sudo=
! command -v sudo 1>/dev/null 2>&1 || sudo=sudo
readonly sudo

readonly _prefix="proofing"
# Which platforms to copy over for images
readonly _platforms="linux/amd64,linux/arm64"
# Certificate lifetime, in days
readonly _lifetime="1"
# Certificates location, relative to pwd
_cert_root="$(pwd)/${_prefix}_certs"
readonly _cert_root
# Registry storage
_storage_root="$(pwd)/${_prefix}_data"
readonly _storage_root
# Name of the registry container
readonly _container_name="${_prefix}"

mkdir -p "$_storage_root"

# x509::reset does remove the local certificates folder
x509::reset(){
  # Remove local cert store
  rm -Rf "$_cert_root"
  mkdir -p "$_cert_root"
}

# x509::ca::new generates a new CA
x509::ca::new(){
  # Gen a CA key
  openssl genrsa -out "$_cert_root/ca.key"

  # Gen a CA pem, valid one day
  openssl req \
    -new \
    -x509 \
    -days "$_lifetime" \
    -subj "/C=US/ST=CA/L=PlanetContainers/O=$_prefix/OU=ci/CN=$_prefix-ca/name=$_prefix CA/emailAddress=$_prefix@example.org" \
    -nameopt compat \
    -key "$_cert_root/ca.key" \
    -out "$_cert_root/ca.pem"

  # Convert to crt
  openssl x509 -outform der -in "$_cert_root"/ca.pem -out "$_cert_root"/ca.crt
}

# x509::cert::new generates and signs a new server certificate with the existing CA, adding as AltName any domain
# present in the file passed as the first argument (along with *.domain), along with localhost, 127.0.0.1, :::1 and
# $_prefix.
x509::cert::new(){
  local alts="$1"
  local alters="DNS:$_prefix,DNS:localhost,IP:127.0.0.1,IP:::1"

  # Gen a server key
  openssl genrsa -out "$_cert_root"/registry.key

  # Gen a csr
  openssl req \
    -new \
    -subj "/C=US/ST=CA/L=PlanetContainers/O=$_prefix/OU=ci/CN=$_prefix-registry/name=$_container_name/emailAddress=$_prefix@example.org" \
    -key "$_cert_root/registry.key" \
    -out "$_cert_root/registry.csr"

  # Add spoofable domains
  while read -r spoof; do
    alters+=",DNS:$spoof,DNS:*.$spoof"
  done <"$alts"

  cat > "$_cert_root/registry-options.cfg" << 'EOF'
basicConstraints=CA:FALSE
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid,issuer
extendedKeyUsage=serverAuth
EOF
  echo "subjectAltName=$alters" >> "$_cert_root/registry-options.cfg"

  # Sign
  openssl x509 \
    -req \
    -days "$_lifetime" \
    -extfile "$_cert_root/registry-options.cfg" \
    -CA "$_cert_root/ca.pem" \
    -CAkey "$_cert_root/ca.key" \
    -nameopt compat \
    -text \
    -in "$_cert_root/registry.csr" \
    -out "$_cert_root/registry.pem"

  # Get a crt
  openssl x509 -outform der -in "$_cert_root/registry.pem" -out "$_cert_root/registry.crt"

  # Create a bundle
  cat "$_cert_root/registry.pem" "$_cert_root/ca.pem" > "$_cert_root/bundle.pem"
}

registry::stop(){
  "$cli" rm -f "$_container_name" 2>/dev/null || true
}

registry::start(){
  local domain
  local short
  local tag
  local digest

  read -r domain short tag digest <"$1"

  "$cli" run -d \
      --restart=always \
      --quiet \
      --name "$_container_name" \
      -p 443:443 \
      --env "OTEL_TRACES_EXPORTER=none" \
      --env "REGISTRY_HTTP_TLS_CERTIFICATE=/registry/domain.pem" \
      --env "REGISTRY_HTTP_TLS_KEY=/registry/domain.key" \
      --env "REGISTRY_HTTP_ADDR=:443" \
      -v "$_cert_root/bundle.pem:/registry/domain.pem" \
      -v "$_cert_root/registry.key:/registry/domain.key" \
      -v "$_storage_root:/var/lib/registry" \
      "$domain/$short:$tag@$digest"
}

host::reset(){
  $sudo rm /usr/local/share/ca-certificates/"$_prefix"* 2>/dev/null || true
  $sudo update-ca-certificates -f
}

host::trust(){
  local ca_file="$1"
  local cert
  local dest

  cert="$(<"$ca_file")"
  dest="$(sha256sum -b <<<"$cert")"
  printf "%s" "$cert" | $sudo tee /usr/local/share/ca-certificates/"$_prefix-${dest:0:16}.crt" >/dev/null
  $sudo update-ca-certificates
}

remote::ca(){
  local ip_or_host="$1"
  local ca

  while read -r line; do
    [ "$line" == "-----BEGIN CERTIFICATE-----" ] && ca="$line"$'\n' || ca+="$line"$'\n'
  done < <(true | openssl s_client -showcerts -connect "$ip_or_host:443" -servername "$_prefix" 2>/dev/null)
  printf "%s\n" "$(echo "$ca" | openssl x509)"
}

host::intercept(){
  local ip="$1"
  local domains="$2"

  $sudo cp /etc/hosts /etc/hosts.backup
  while read -r spoof; do
    $sudo echo "$ip $spoof # -- mark $_prefix --" | $sudo tee -a /etc/hosts >/dev/null
  done <"$domains"
}

host::deintercept(){
  [ ! -e /etc/hosts.backup ] || $sudo mv /etc/hosts.backup /etc/hosts
}

registry::seed(){
  local images="$1"
  local platforms="$2"
  local domain
  local short
  local tag
  local digest
  local fqim

  registry="$_prefix:443"
  while read -r domain short tag digest; do
    fqim="$domain/$short:$tag@$digest"
    echo "Pulling $fqim"
    "$cli" pull --quiet --platform="$platforms" "$fqim"
    echo "Tagging $registry/$short:$tag"
    "$cli" tag "$fqim" "$registry/$short:$tag"
    echo "Pushing $registry/$short:$tag"
    "$cli" push --quiet --platform="$platforms" "$registry/$short:$tag"
  done <"$images"
}

case "$1" in
  server::reset)
    # Stop registry
    registry::stop
    # Delete certificates
    x509::reset
    # Un-trust prior CAs
    host::reset
    # Remove the domain
    host::deintercept
    ;;
  server::start)
    env_file="$2"

    # Stop registry
    registry::stop
    # Delete certificates
    x509::reset
    # Un-trust prior CAs
    host::reset
    # Remove the domain
    host::deintercept

    # Create new CA
    x509::ca::new
    # Create new server cert
    x509::cert::new <("$root"/forward.sh domains "$env_file")
    # Start the registry
    registry::start <("$root"/forward.sh images "$env_file" | grep "library/registry")
    # Trust the CA locally
    host::trust <(remote::ca "localhost")
    # Add the domain
    host::intercept "127.0.0.1" <(printf "%s\n" "$_prefix")
    ;;
  server::seed)
    env_file="$2"

    registry::seed <("$root"/forward.sh images "$env_file") "$_platforms"
    ;;
  guest::reset)
    env_file="$2"
    ip="$3"

    host::reset
    host::deintercept
    ;;
  guest::configure)
    env_file="$2"
    ip="${3:-$(cat /etc/hosts | grep "$_prefix" | awk '{print $1}')}"
    host::reset
    host::deintercept

    host::intercept "$ip" <("$root"/forward.sh domains "$env_file")
    host::trust <(remote::ca "$ip")
    ;;

  *)
    echo "unknown command"
    exit 1
    ;;
esac
