#!/bin/sh
# Install envcraft and print where the binary landed.
# go install itself is silent, so this wrapper is the confirmation.
set -eu

module=github.com/Ahmadhaff/envcraft
version=${ENVCRAFT_VERSION:-latest}

if ! command -v go >/dev/null 2>&1; then
	echo "Go 1.26 or newer is required to install envcraft." >&2
	exit 1
fi

echo "Installing ${module}@${version}"
go install "${module}@${version}"

gobin=$(go env GOBIN)
if [ -z "${gobin}" ]; then
	gobin="$(go env GOPATH)/bin"
fi

name=envcraft
if [ "$(go env GOOS)" = "windows" ]; then
	name=envcraft.exe
fi
bin="${gobin}/${name}"

if [ ! -x "${bin}" ]; then
	echo "Install finished, but ${bin} is missing." >&2
	exit 1
fi

printf '\n'
printf 'envcraft installed\n\n'
printf '  binary  %s\n' "${bin}"

case ":${PATH}:" in
*":${gobin}:"*)
	printf '  ready   envcraft --help\n'
	;;
*)
	printf '  PATH    %s is not on PATH yet\n' "${gobin}"
	printf '          Add this to your shell config, then open a new terminal:\n'
	printf '          export PATH="%s:$PATH"\n' "${gobin}"
	;;
esac
printf '\n'
