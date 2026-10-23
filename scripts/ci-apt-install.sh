#!/usr/bin/env bash
# Installs Debian or Ubuntu packages in a CI job's container. apt retries every download, and when the image's mirror
# stays unreachable (the Playwright image uses azure.archive.ubuntu.com) the install moves to Ubuntu's primary mirrors.
# Usage: ci-apt-install.sh <package>...
set -euo pipefail
[ $# -gt 0 ] || { echo "usage: ci-apt-install.sh <package>..." >&2; exit 2; }

installPackages() {
  apt-get -qq -o Acquire::Retries=3 update &&
    DEBIAN_FRONTEND=noninteractive apt-get -qq -o Acquire::Retries=3 install -y "$@" > /dev/null
}

installPackages "$@" && exit 0
echo "::warning::apt could not use the image's mirror; retrying with archive.ubuntu.com and ports.ubuntu.com"
find /etc/apt -type f \( -name '*.list' -o -name '*.sources' \) -exec sed -i \
  -e 's|http://azure\.archive\.ubuntu\.com|http://archive.ubuntu.com|g' \
  -e 's|http://azure\.ports\.ubuntu\.com|http://ports.ubuntu.com|g' {} +
installPackages "$@"
