#!/bin/sh

set -e

TAG="v4.4.3"

CHECKSUMS="
003191c02d1d80a9b6ce4144d7bb242db6de365ce0fa8c42efa71a2aba7aa9ab  bindown_4.4.3_windows_arm64.tar.gz
3ad0dc8aa646eb5602b47d538179a2e1733102374022188c2fb2b9b476d3c3aa  bindown_4.4.3_darwin_arm64
615217bca6577c84ee57bc54b09ebf888da637fdcaa4495c745e64946f9b3e21  bindown_4.4.3_linux_arm64.tar.gz
6195963d8e74f1c8382a829c6450a6684d44d9a33e8dc5b86d2d9ea2183d13ad  bindown_4.4.3_darwin_amd64
71288401bca867167c77c46f8f7eba7a5524bfb34a9725adcf8a350d1d555189  bindown_4.4.3_linux_amd64.tar.gz
801740a487d9c38fa296d27c6a721ebc8e6f4e1b57a6a4366b8dc923c10f484f  bindown_4.4.3_windows_amd64.tar.gz
953873a00b0115e9c64d3b710c0f3591628691f6d731423d718e0013b0bc14c0  bindown_4.4.3_linux_amd64
9890704a45c64ad455eb1c2ebe09f4384f565004dd17c67d1c47a4cb5d25d5af  bindown_4.4.3_linux_arm64
995e3f85edb53179f0b5fddab623651935c7e875814343b6ebfb2ed85c2b3133  bindown_4.4.3_linux_386.tar.gz
c9fef9ba77a79bd6caa51e06121a98c0ab089b8e7082bc518a7a0f17010a8f0a  bindown_4.4.3_windows_386.exe
d77fffb01d1f6926e677e1871549c29fe479c58c085c48b56c86b7c4fef53559  bindown_4.4.3_windows_386.tar.gz
dd6a38f53a4ad990ae8d85e02a5c1bf6ea1cf37bb5a5e2bc8d4ade8d6e10c089  bindown_4.4.3_linux_386
de80f88620e7b943e6c5e751b629d00b00790b14213ea8543ed0a4ac548f17db  bindown_4.4.3_windows_arm64.exe
e518ad20be3f186b5e5a4425f87a019b5c5f13a63824b84bddd62c0b86e1d2c3  bindown_4.4.3_windows_amd64.exe
ee0c4abac8358654d73df5f9dc3247c8a4a09f3df9805a36ade7c8720b99db41  bindown_4.4.3_darwin_amd64.tar.gz
f4b7501c313b9019845425e16770769f79fea7577f466794e6c6561aaad80d16  bindown_4.4.3_darwin_arm64.tar.gz
"

cat /dev/null << EOF
------------------------------------------------------------------------
https://github.com/client9/shlib - portable posix shell functions
Public domain - http://unlicense.org
https://github.com/client9/shlib/blob/master/LICENSE.md
but credit (and pull requests) appreciated.
------------------------------------------------------------------------
EOF
is_command() {
  command -v "$1" > /dev/null
}
echoerr() {
  echo "$@" 1>&2
}
log_prefix() {
  echo "$0"
}
_logp=6
log_set_priority() {
  _logp="$1"
}
log_priority() {
  if test -z "$1"; then
    echo "$_logp"
    return
  fi
  [ "$1" -le "$_logp" ]
}
log_tag() {
  case $1 in
    0) echo "emerg" ;;
    1) echo "alert" ;;
    2) echo "crit" ;;
    3) echo "err" ;;
    4) echo "warning" ;;
    5) echo "notice" ;;
    6) echo "info" ;;
    7) echo "debug" ;;
    *) echo "$1" ;;
  esac
}
log_debug() {
  log_priority 7 || return 0
  echoerr "$(log_prefix)" "$(log_tag 7)" "$@"
}
log_info() {
  log_priority 6 || return 0
  echoerr "$(log_prefix)" "$(log_tag 6)" "$@"
}
log_err() {
  log_priority 3 || return 0
  echoerr "$(log_prefix)" "$(log_tag 3)" "$@"
}
log_crit() {
  log_priority 2 || return 0
  echoerr "$(log_prefix)" "$(log_tag 2)" "$@"
}
uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    cygwin_nt*) os="windows" ;;
    mingw*) os="windows" ;;
    msys_nt*) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo "${arch}"
}
uname_os_check() {
  os=$(uname_os)
  case "$os" in
    darwin) return 0 ;;
    dragonfly) return 0 ;;
    freebsd) return 0 ;;
    linux) return 0 ;;
    android) return 0 ;;
    nacl) return 0 ;;
    netbsd) return 0 ;;
    openbsd) return 0 ;;
    plan9) return 0 ;;
    solaris) return 0 ;;
    windows) return 0 ;;
  esac
  log_crit "uname_os_check '$(uname -s)' got converted to '$os' which is not a GOOS value. Please file bug at https://github.com/client9/shlib"
  return 1
}
uname_arch_check() {
  arch=$(uname_arch)
  case "$arch" in
    386) return 0 ;;
    amd64) return 0 ;;
    arm64) return 0 ;;
    armv5) return 0 ;;
    armv6) return 0 ;;
    armv7) return 0 ;;
    ppc64) return 0 ;;
    ppc64le) return 0 ;;
    mips) return 0 ;;
    mipsle) return 0 ;;
    mips64) return 0 ;;
    mips64le) return 0 ;;
    s390x) return 0 ;;
    amd64p32) return 0 ;;
  esac
  log_crit "uname_arch_check '$(uname -m)' got converted to '$arch' which is not a GOARCH value.  Please file bug report at https://github.com/client9/shlib"
  return 1
}
untar() {
  tarball=$1
  case "${tarball}" in
    *.tar.gz | *.tgz) tar --no-same-owner -xzf "${tarball}" ;;
    *.tar) tar --no-same-owner -xf "${tarball}" ;;
    *.zip) unzip "${tarball}" ;;
    *)
      log_err "untar unknown archive format for ${tarball}"
      return 1
      ;;
  esac
}
http_download_curl() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    code=$(curl -w '%{http_code}' -sL -o "$local_file" "$source_url")
  else
    code=$(curl -w '%{http_code}' -sL -H "$header" -o "$local_file" "$source_url")
  fi
  if [ "$code" != "200" ]; then
    log_debug "http_download_curl received HTTP status $code"
    return 1
  fi
  return 0
}
http_download_wget() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    wget -q -O "$local_file" "$source_url"
  else
    wget -q --header "$header" -O "$local_file" "$source_url"
  fi
}
http_download() {
  log_debug "http_download $2"
  if is_command curl; then
    http_download_curl "$@"
    return
  elif is_command wget; then
    http_download_wget "$@"
    return
  fi
  log_crit "http_download unable to find wget or curl"
  return 1
}
hash_sha256() {
  TARGET=${1:-/dev/stdin}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$TARGET" 2> /dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl -dst openssl dgst -sha256 "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f a
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}
hash_sha256_verify() {
  TARGET=$1
  checksums=$2
  if [ -z "$checksums" ]; then
    log_err "hash_sha256_verify checksum file not specified in arg2"
    return 1
  fi
  BASENAME=${TARGET##*/}
  want=$(grep "${BASENAME}" "${checksums}" 2> /dev/null | tr '\t' ' ' | cut -d ' ' -f 1)
  if [ -z "$want" ]; then
    log_err "hash_sha256_verify unable to find checksum for '${TARGET}' in '${checksums}'"
    return 1
  fi
  got=$(hash_sha256 "$TARGET")
  if [ "$want" != "$got" ]; then
    log_err "hash_sha256_verify checksum for '$TARGET' did not verify ${want} vs $got"
    return 1
  fi
}
cat /dev/null << EOF
------------------------------------------------------------------------
End of functions from https://github.com/client9/shlib
------------------------------------------------------------------------
EOF

FORMAT=tar.gz
GITHUB_DOWNLOAD=https://github.com/WillAbides/bindown/releases/download

usage() {
  this=$1
  cat << EOT
Usage: $this [-b bindir] [-d]

Usage: $this [-b] bindir [-d]
  -b sets bindir or installation directory, Defaults to ./bin
  -d turns on debug logging

EOT
  exit 2
}

parse_args() {
  #BINDIR is ./bin unless set be ENV
  # over-ridden by flag below

  BINDIR=${BINDIR:-./bin}
  while getopts "b:dh?x" arg; do
    case "$arg" in
      b) BINDIR="$OPTARG" ;;
      d) log_set_priority 10 ;;
      h | \?) usage "$0" ;;
      x) set -x ;;
    esac
  done
  shift $((OPTIND - 1))
}

execute() {
  tmpdir=$(mktemp -d)
  echo "$CHECKSUMS" > "${tmpdir}/checksums.txt"
  log_debug "downloading files into ${tmpdir}"
  http_download "${tmpdir}/${TARBALL}" "${TARBALL_URL}"
  hash_sha256_verify "${tmpdir}/${TARBALL}" "${tmpdir}/checksums.txt"
  srcdir="${tmpdir}"
  (cd "${tmpdir}" && untar "${TARBALL}")
  test ! -d "${BINDIR}" && install -d "${BINDIR}"
  binexe="bindown"
  if [ "$OS" = "windows" ]; then
    binexe="${binexe}.exe"
  fi
  install "${srcdir}/${binexe}" "${BINDIR}/"
  log_info "installed ${BINDIR}/${binexe}"
  rm -rf "${tmpdir}"
}

OS=$(uname_os)
ARCH=$(uname_arch)

uname_os_check "$OS"
uname_arch_check "$ARCH"

parse_args "$@"

VERSION=${TAG#v}
NAME=bindown_${VERSION}_${OS}_${ARCH}
TARBALL=${NAME}.${FORMAT}
TARBALL_URL=${GITHUB_DOWNLOAD}/${TAG}/${TARBALL}

execute
