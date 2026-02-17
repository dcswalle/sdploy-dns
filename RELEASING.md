# Release Process

This document explains how to create a new release of `go-dns` using the automated release workflow.

## Overview

The project uses GitHub Actions to automate the build and release process. The workflow is defined in `.github/workflows/release.yml` and is triggered when you push a version tag to the repository.

## Creating a Release

### Prerequisites

- Ensure all changes are committed and pushed to the `main` branch
- Ensure all tests pass and the code is ready for release
- Decide on the version number following [Semantic Versioning](https://semver.org/)

### Version Numbering

Use semantic versioning (MAJOR.MINOR.PATCH):
- **MAJOR**: Breaking changes or major new features
- **MINOR**: New features that are backward compatible
- **PATCH**: Bug fixes and minor improvements

Examples: `v1.0.0`, `v1.2.3`, `v2.0.0-beta.1`

### Steps

1. **Create and push a version tag**:

   ```bash
   git checkout main
   git pull origin main
   
   # Create the tag (replace with your version)
   git tag -a v1.0.0 -m "Release v1.0.0"
   
   # Push the tag to GitHub
   git push origin v1.0.0
   ```

2. **Monitor the workflow**:

   - Go to the [Actions tab](https://github.com/dcswalle/sdploy-dns/actions) in your repository
   - You should see the "Build & Release" workflow running
   - The workflow will:
     - Build binaries for multiple platforms (Linux and macOS, both amd64 and arm64)
     - Generate release notes from commit messages since the last tag
     - Create a GitHub release with the built binaries attached

3. **Verify the release**:

   - Once the workflow completes, check the [Releases page](https://github.com/dcswalle/sdploy-dns/releases)
   - The new release should be published with:
     - Release notes (auto-generated from commits)
     - Binary artifacts for all platforms:
       - `go-dns-linux-amd64`
       - `go-dns-linux-arm64`
       - `go-dns-darwin-amd64`
       - `go-dns-darwin-arm64`

## What the Workflow Does

The release workflow (`.github/workflows/release.yml`) performs the following:

### Build Job

- Runs a matrix build for multiple platforms (Linux and macOS) and architectures (amd64 and arm64)
- Uses Go 1.24 with CGO disabled for static binaries
- Embeds the version string from the tag name into the binary using `-ldflags`
- Uploads each binary as a build artifact

### Release Job

- Downloads all build artifacts
- Generates release notes from commit messages since the previous tag
- Creates a GitHub release with:
  - The tag name as the release title
  - Auto-generated release notes
  - All binary artifacts attached
  - Automatically marks releases as "prerelease" if the tag contains a hyphen (e.g., `v1.0.0-beta.1`)

## Pre-releases

To create a pre-release (beta, alpha, release candidate), include a hyphen in the tag name:

```bash
git tag -a v1.0.0-beta.1 -m "Beta release v1.0.0-beta.1"
git push origin v1.0.0-beta.1
```

The workflow will automatically mark this as a pre-release on GitHub.

## Troubleshooting

### Workflow Doesn't Trigger

- Ensure the tag starts with `v` (e.g., `v1.0.0`, not `1.0.0`)
- Check that you pushed the tag: `git push origin <tag-name>`
- Verify the workflow file exists at `.github/workflows/release.yml`

### Build Fails

- Check the workflow logs in the Actions tab
- Ensure the code compiles locally: `go build .`
- Verify Go version compatibility (requires Go 1.24+)

### Missing Binaries

- Check that all build jobs completed successfully in the Actions tab
- Verify artifacts were uploaded (visible in the workflow run details)
- Ensure the release job successfully downloaded artifacts

## Deleting a Release

If you need to delete a release:

1. Delete the release from the GitHub UI (Releases page)
2. Delete the tag locally and remotely:
   ```bash
   git tag -d v1.0.0
   git push origin :refs/tags/v1.0.0
   ```
3. Fix any issues and create a new tag/release

## Manual Release (Not Recommended)

If you need to create a release manually without the workflow:

```bash
# Build for all platforms
GOOS=linux GOARCH=amd64 go build -o go-dns-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o go-dns-linux-arm64 .
GOOS=darwin GOARCH=amd64 go build -o go-dns-darwin-amd64 .
GOOS=darwin GOARCH=arm64 go build -o go-dns-darwin-arm64 .
```

Then create the release manually through the GitHub UI. However, using the automated workflow is strongly recommended for consistency.
