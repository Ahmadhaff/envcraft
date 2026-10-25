# envcraft

`envcraft` is a local development CLI for three jobs: locking `.env` files, checking a stack concurrently, and scaffolding Clean Architecture projects.

## Install

Requires Go 1.26 or newer. Current Cobra, Viper, and Charm releases set that floor. Generated Go projects use the same version and the standard library.

`go install` does not print anything when it succeeds, and it does not add an entry to `package.json`. The installer runs that command and then confirms the binary path:

```bash
curl -fsSL https://raw.githubusercontent.com/Ahmadhaff/envcraft/main/scripts/install.sh | sh
```

From a checkout of this repository:

```bash
sh scripts/install.sh
```

The binary is installed into `$(go env GOPATH)/bin`, usually `~/go/bin`. If that directory is not on `PATH`, the installer prints the line to add.

To build from a checkout without using the module proxy:

```bash
go build -o envcraft .
go install .
```

Cross-compile release archives with [GoReleaser](https://goreleaser.com):

```bash
goreleaser release --snapshot --clean
```

That builds macOS amd64 and arm64, Linux amd64 and arm64, and Windows amd64 and arm64.

## Commands

```bash
envcraft init
envcraft doctor
envcraft env lock
envcraft env unlock
envcraft env status
envcraft generate go --name ledger --module github.com/acme/ledger
envcraft generate node --name web
```

`init` reads the current directory and writes `envcraft.yaml`. It uses the first Compose file it finds (`compose.yaml` or `docker-compose.yml`), the `go` line in `go.mod`, the `engines.node` range in `package.json`, and the keys in `.env.example`.

`doctor` (`check`) runs every probe at once. The summary is wall-clock time. Each row is that check's own latency. When `envcraft.yaml` is a project contract, doctor checks that contract: published Compose ports, the Go or Node version the project asks for, and the required env keys. A local URL in `.env`, such as `postgres://127.0.0.1:5432/app`, is dialed. A URL that points outside the machine is left alone. Without a project contract, doctor keeps the built-in list:

| Check | What it means |
| --- | --- |
| Docker | `docker info` can reach a daemon |
| PostgreSQL | TCP connect to the configured port (default 5432) |
| Redis | TCP connect to the configured port (default 6379) |
| Node.js | `node` is on `PATH` |
| Go | `go` is on `PATH` |

Port rows report reachability, not an authenticated session. A failed check exits `1`. Canceling exits `130`.

`generate` (`gen`) writes a small Clean Architecture tree plus `.env.example` and `envcraft.yaml`. The Go tree also includes a Compose file for Postgres. With no flags in a terminal it asks for the stack, name, and Go module. `go` and `node` (alias `ts`) are the built-in stacks. The Go service is standard-library only and includes a test. Its HTTP adapter stores users in memory. `DATABASE_URL` is the local database the contract checks.

## Vaults

`env lock` seals a `.env` file as `.env.vault`. `env unlock` reverses it. The source file is left in place. Both outputs are mode `0600`.

Version 1 layout:

```text
envcraft-vault 1
base64(salt || nonce || ciphertext)
```

- AES-256-GCM, 12 byte nonce, 16 byte salt, fresh values on every lock
- scrypt parameters `N=32768`, `r=8`, `p=1`
- GCM additional data `envcraft-vault-v1`
- Passphrase of at least 8 characters when locking

Passphrase sources, in order:

1. `--passphrase` (visible in the shell history and process list)
2. `ENVCRAFT_PASSPHRASE`
3. An interactive prompt

The passphrase is never read from `envcraft.yaml`. A wrong passphrase and a tampered vault both fail authentication. Refusing to double-lock a file that is already a vault avoids wrapping ciphertext by mistake. This is a local format, not a hosted secrets manager.

## Configuration

Copy `config.example.yaml` to `./envcraft.yaml` or `$XDG_CONFIG_HOME/envcraft/envcraft.yaml` (on macOS, `~/Library/Application Support/envcraft/envcraft.yaml`).

Flags win over environment variables, which win over the file.

| Setting | Environment | Default |
| --- | --- | --- |
| `doctor.host` | `ENVCRAFT_DOCTOR_HOST` | `127.0.0.1` |
| `doctor.postgres_port` | `ENVCRAFT_DOCTOR_POSTGRES_PORT` | `5432` |
| `doctor.redis_port` | `ENVCRAFT_DOCTOR_REDIS_PORT` | `6379` |
| `doctor.timeout` | `ENVCRAFT_DOCTOR_TIMEOUT` | `2s` |
| `env.file` | `ENVCRAFT_ENV_FILE` | `.env` |
| `env.vault` | `ENVCRAFT_ENV_VAULT` | `.env.vault` |

`--debug` prints each probe target. `--no-color` and `NO_COLOR` disable color, as does a non-terminal stdout. `--config` selects a file explicitly.

## Development

```bash
gofmt -l .
go test -race ./...
```

Version, commit, and date are set at link time:

```bash
go build -ldflags "-X github.com/Ahmadhaff/envcraft/cmd.version=0.1.0 -X github.com/Ahmadhaff/envcraft/cmd.commit=abc -X github.com/Ahmadhaff/envcraft/cmd.date=2026-10-25" .
```

## Layout

```text
cmd/                              cobra commands and terminal styles
internal/checker/                 concurrent probes
internal/crypto/                  AES-256-GCM vaults
internal/contract/                project discovery and doctor contracts
internal/scaffolder/              embedded template engine
internal/scaffolder/templates/    Go and Node Clean Architecture trees
```
