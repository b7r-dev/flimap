# Release Process

## Versioning

flimap uses [semantic versioning](https://semver.org/) with git tags:

- **v0.MINOR.PATCH** during 0.x (pre-1.0)
  - MINOR: new features or breaking changes
  - PATCH: bug fixes
- **v1.0.0+**: stable API contract
  - MAJOR: breaking changes
  - MINOR: new features (backwards compatible)
  - PATCH: bug fixes (backwards compatible)

The version is injected at build time via `-ldflags` into the `main.version` variable. Local builds default to `"dev"`. Release builds get the tag version (e.g. `0.1.0`) stamped by GoReleaser.

## Releasing

Releases are fully automated via GitHub Actions + GoReleaser. Pushing a tag triggers the workflow.

### Steps

```sh
# 1. Make sure main is up to date and builds clean
git checkout main
git pull origin main
go build ./... && go vet ./...

# 2. Create an annotated tag
git tag -a v0.X.Y -m "v0.X.Y — short description of what changed"

# 3. Push the tag
git push origin v0.X.Y
```

### What happens automatically

1. The `Release` GitHub Action triggers on the `v*` tag
2. GoReleaser cross-compiles for:
   - `darwin/arm64` (Apple Silicon)
   - `darwin/amd64` (Intel Macs)
   - `linux/amd64`
   - `linux/arm64`
3. Each binary is archived as `flimap_<version>_<os>_<arch>.tar.gz`
4. A `checksums.txt` file is generated
5. A GitHub Release is created with an auto-generated changelog from commit history

The release appears at `https://github.com/b7r-dev/flimap/releases/tag/v0.X.Y`.

### After releasing

- Verify the release assets on GitHub
- Update the OpenCode or Claude Desktop config if the binary path changed
- Share the release link if appropriate

## Installation Methods

Users can install a specific version three ways:

```sh
# Go install (requires Go installed)
go install github.com/b7r-dev/flimap@v0.X.Y

# Download prebuilt binary
curl -L https://github.com/b7r-dev/flimap/releases/download/v0.X.Y/flimap_v0.X.Y_macOS_arm64.tar.gz | tar xz

# Build from source
git clone https://github.com/b7r-dev/flimap.git
cd flimap
git checkout v0.X.Y
go build -o flimap
```

## GoReleaser Configuration

The release pipeline is configured in `.goreleaser.yml` and `.github/workflows/release.yml`. Key settings:

- `CGO_ENABLED=0` — pure Go, no C dependencies, clean cross-compilation
- `-s -w` ldflags — stripped binaries, smaller size
- Version injected via `-X main.version={{.Version}}`
- Archives named with human-readable OS/arch (`macOS_arm64`, `linux_x86_64`)
