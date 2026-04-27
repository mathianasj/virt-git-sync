# Release Process

## Prerequisites

### GitHub Secrets Setup

The release workflow requires the following secrets to be configured in your GitHub repository:

1. Go to GitHub repository → Settings → Secrets and variables → Actions
2. Add the following secrets:

#### Required Secrets

- **QUAY_USERNAME**: Your Quay.io username (e.g., `mathianasj`)
- **QUAY_TOKEN**: Your Quay.io Robot Account token or password

### Creating a Quay.io Robot Account (Recommended)

1. Go to https://quay.io/repository/mathianasj/virt-git-sync
2. Click on the repository (create it if it doesn't exist)
3. Go to "Settings" → "Robot Accounts"
4. Create a new robot account with "Write" permissions
5. Copy the token and add it as `QUAY_TOKEN` in GitHub secrets

Alternatively, you can use your Quay.io password directly, but a robot account is more secure.

## Creating a Release

### 1. Ensure main/master is ready

```bash
# Make sure all changes are committed and pushed
git status
git push origin master
```

### 2. Create and push a version tag

```bash
# Create a new version tag (use semantic versioning)
git tag v1.0.0

# Push the tag to GitHub
git push origin v1.0.0
```

### 3. Automated release process

Once you push a tag matching `v*.*.*`, GitHub Actions will automatically:

1. ✅ Run all tests (unit tests + auto-pause tests)
2. ✅ Run linting checks
3. ✅ Build multi-arch Docker image (linux/amd64 + linux/arm64)
4. ✅ Push image to `quay.io/mathianasj/virt-git-sync:v1.0.0`
5. ✅ Push image to `quay.io/mathianasj/virt-git-sync:latest`
6. ✅ Generate installation manifests
7. ✅ Create GitHub Release with:
   - Auto-generated release notes
   - Installation instructions
   - Complete install.yaml manifest
   - CRD manifests

### 4. Verify release

1. Check GitHub Actions: https://github.com/mathianasj/virt-git-sync/actions
2. Check GitHub Releases: https://github.com/mathianasj/virt-git-sync/releases
3. Check Quay.io: https://quay.io/repository/mathianasj/virt-git-sync

## Version Numbering

Follow semantic versioning (semver):

- **MAJOR.MINOR.PATCH** (e.g., `v1.2.3`)
- **MAJOR**: Breaking changes, API incompatibilities
- **MINOR**: New features, backward-compatible
- **PATCH**: Bug fixes, backward-compatible

Examples:
- `v1.0.0` - Initial release
- `v1.1.0` - Added new feature (auto-pause)
- `v1.1.1` - Bug fix in auto-pause
- `v2.0.0` - Breaking change (removed local-only mode)

## Pre-releases

For alpha/beta releases, use tags like:
```bash
git tag v1.0.0-alpha.1
git tag v1.0.0-beta.1
git tag v1.0.0-rc.1
```

The workflow will create the release but mark it as a pre-release automatically.

## Manual Release (if needed)

If the automated workflow fails, you can manually build and push:

```bash
# Build and push image
export VERSION=v1.0.0
export IMG=quay.io/mathianasj/virt-git-sync:$VERSION

docker login quay.io
make docker-build docker-push IMG=$IMG

# Generate manifests
make build-installer IMG=$IMG

# Create GitHub release manually and upload dist/install.yaml
```

## Troubleshooting

### Workflow fails with "unauthorized" when pushing to Quay.io

- Verify `QUAY_USERNAME` and `QUAY_TOKEN` secrets are set correctly
- Verify the robot account has "Write" permissions
- Check if the repository exists on Quay.io and is not set to private

### Image fails to build

- Check the Dockerfile exists and is valid
- Check Go version in workflow matches go.mod
- Review build logs in GitHub Actions

### Tests fail during release

- Run tests locally first: `make test && make test-auto-pause`
- Check test logs in GitHub Actions
- Fix issues and create a new tag
