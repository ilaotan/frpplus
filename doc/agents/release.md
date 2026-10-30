# Release Process

## 1. Update Release Notes

Edit `Release.md` in the project root with the changes for this version:

```markdown
## Features
* ...

## Improvements
* ...

## Fixes
* ...
```

This file is used by GoReleaser as the GitHub Release body.

## 2. Bump Version

Update the version string in `pkg/util/version/version.go`:

```go
var version = "0.X.0"
```

Commit and push to `dev`:

```bash
git add pkg/util/version/version.go Release.md
git commit -m "bump version to vX.Y.Z"
git push origin dev
```

## 3. Pre-release Validation

Run the standard e2e suite locally:

```bash
make e2e
```

For releases that touch compatibility-sensitive areas such as login, control
connections, work connections, visitors, transport, or wire protocol handling,
also run the manual compatibility e2e suite:

```bash
make e2e-compatibility
make e2e-compatibility-floor
```

`make e2e-compatibility` builds the current `frps` and `frpc`, resolves the
recent stable release baselines from GitHub, downloads or reuses their binaries,
and tests current binaries against those historical releases. The default number
of recent baselines is controlled by `FRP_COMPAT_BASELINE_COUNT` in the
`Makefile`.

Downloaded release binaries are cached under:

```text
.cache/e2e-compat/<version>/<os>_<arch>/
```

For a release validation run that must be exactly reproducible, pass an explicit
baseline matrix instead of using the floating recent-release list:

```bash
FRP_COMPAT_BASELINE_VERSIONS="0.X.0 0.Y.0" make e2e-compatibility
```

Use `make e2e-compatibility-smoke` for a quick single-baseline check while
iterating locally. If GitHub release metadata requests are rate-limited, set
`GITHUB_TOKEN` or use `FRP_COMPAT_BASELINE_VERSIONS`.

The compatibility floor is a support-policy decision, not a value that should
change every release. Update `FRP_COMPAT_FLOOR_VERSION` only when the declared
compatibility window changes.

## 4. Merge dev → master

Create a PR from `dev` to `master`:

```bash
gh pr create --base master --head dev --title "bump version"
```

Wait for CI to pass, then merge using **merge commit** (not squash).

## 5. Tag the Release

```bash
git checkout master
git pull origin master
git tag -a vX.Y.Z -m "bump version"
git push origin vX.Y.Z
```

## 6. Trigger GoReleaser

Manually trigger the `goreleaser` workflow in GitHub Actions:

```bash
gh workflow run goreleaser --ref master
```

GoReleaser will:
1. Run `package.sh` to cross-compile all platforms and create archives
2. Create a GitHub Release with all packages, using `Release.md` as release notes

## Key Files

| File | Purpose |
|------|---------|
| `pkg/util/version/version.go` | Version string |
| `Release.md` | Release notes (read by GoReleaser) |
| `.goreleaser.yml` | GoReleaser config |
| `package.sh` | Cross-compile and packaging script |
| `.github/workflows/goreleaser.yml` | GitHub Actions workflow (manual trigger) |

## Versioning

- Minor release: `v0.X.0`
- Patch release: `v0.X.Y` (e.g., `v0.62.1`)

## Fork Builds (this repository)

`pkg/util/version/version.go` is kept identical to upstream on purpose, the fork
build number is injected at build time, so a binary reports
`frp_<upstream version>.plus.<n>` (for example `frp_0.71.1.plus.3`).

```bash
bash hack/build-release.sh -b 3         # build number, required
bash hack/build-release.sh -b 3 --no-web # keep the existing web dist
bash hack/build-release.sh -b 3 --no-upx # keep the binaries uncompressed
bash hack/build-release.sh -b 3 -p "linux/amd64 linux/arm64"
```

It cross compiles `windows/amd64`, `linux/amd64`, `linux/arm` (GOARM=7) and
`linux/arm64` with `CGO_ENABLED=0 -trimpath -ldflags "-s -w"`, packs one zip per
platform into `bin/` (staged trees stay in `release/pack/`), checks the version
reported by the host binary and prints the sha256 of every package.

The binaries are compressed with UPX when it is available, which cuts them to
roughly 30% of their original size. Set `UPX_BIN` to point at a specific `upx`
executable, or pass `--no-upx` to ship them uncompressed.

The binaries inside a package are named `frpsplustiny` and `frpcplustiny` (with
`.exe` on windows), the archive is
`frpsplustiny_<version>_<os>_<arch>.zip` and the folder inside it carries the
same name:

```
frpsplustiny_frp_0.71.1.plus.3_linux_amd64/
├── frpsplustiny
├── frpcplustiny
├── frps.example.toml
├── frpc.example.toml
└── LICENSE
```
