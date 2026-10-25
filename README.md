# envcraft

`envcraft` is a local development CLI for three jobs: locking `.env` files, checking a stack concurrently, and scaffolding Clean Architecture projects.

## Install

Requires Go 1.26 or newer. Current Cobra, Viper, and Charm releases set that floor. The scaffolds themselves stay on Go 1.22 and the standard library.

```bash
go build -o envcraft .
./envcraft --help
```

Install into your `GOBIN` from this checkout:

```bash
go install .
```

Cross-compile release archives with [GoReleaser](https://goreleaser.com):

```bash
goreleaser release --snapshot --clean
```

That builds macOS amd64 and arm64, Linux amd64 and arm64, and Windows amd64 and arm64.

## Commands

```bash
envcraft doctor
envcraft env lock
envcraft env unlock
envcraft env status
envcraft generate go --name ledger --module github.com/acme/ledger
envcraft generate node --name web
```

`doctor` (`check`) runs every probe at once. The summary is wall-clock time. Each row is that check's own latency.

| Check | What it means |
| --- | --- |
| Docker | `docker info` can reach a daemon |
| PostgreSQL | TCP connect to the configured port (default 5432) |
| Redis | TCP connect to the configured port (default 6379) |
| Node.js | `node` is on `PATH` |
| Go | `go` is on `PATH` |

Port rows report reachability, not an authenticated session. A failed check exits `1`. Canceling exits `130`.

`generate` (`gen`) writes a small Clean Architecture tree. With no flags in a terminal it asks for the stack, name, and Go module. `go` and `node` (alias `ts`) are the built-in stacks. The Go scaffold is standard-library only and includes a test.

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
go build -ldflags "-X github.com/ahmadhaffoudhi/envcraft/cmd.version=0.1.0 -X github.com/ahmadhaffoudhi/envcraft/cmd.commit=abc -X github.com/ahmadhaffoudhi/envcraft/cmd.date=2026-10-25" .
```

## Layout

```text
cmd/                 cobra commands and terminal styles
internal/checker/    concurrent probes
internal/crypto/     AES-256-GCM vaults
internal/scaffolder/ template rendering
templates/           embedded Go and Node trees
```
