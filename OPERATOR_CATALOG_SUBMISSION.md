# Red Hat Community Operators Catalog Submission Guide

## Automated Submission (Recommended)

**GitHub Actions automatically creates PRs to operator catalogs when you publish a release!**

### Prerequisites

- [x] Operator bundle created (bundle/ directory exists)
- [x] ClusterServiceVersion (CSV) with proper metadata
- [x] Container image published to public registry (quay.io/mathianasj/virt-git-sync:v0.1.0)
- [ ] Icon updated in CSV (currently using custom SVG icon)
- [x] Multi-arch images built (amd64, arm64)
- [x] CI tests passing on GitHub
- [x] Documentation complete
- [ ] GitHub Personal Access Token configured (see Setup below)

### Setup (One-time)

1. **Create GitHub Personal Access Token:**
   - Go to https://github.com/settings/tokens/new
   - Name: "OperatorHub Catalog Submissions"
   - Expiration: No expiration (or set a long duration)
   - Scopes required:
     - ✅ `public_repo` - Access public repositories
     - ✅ `workflow` - Update GitHub Action workflows
   - Click "Generate token" and copy it

2. **Add token to repository secrets:**
   - Go to https://github.com/mathianasj/virt-git-sync/settings/secrets/actions
   - Click "New repository secret"
   - Name: `OPERATOR_CATALOG_TOKEN`
   - Value: (paste the token you copied)
   - Click "Add secret"

3. **Fork the catalog repositories** (one-time):
   - Fork https://github.com/k8s-operatorhub/community-operators
   - Fork https://github.com/redhat-openshift-ecosystem/community-operators-prod

### Publishing a Release

When you create a new release tag, automation handles everything:

```bash
# Create and push a release tag
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions will:
1. ✅ Build and push multi-arch images
2. ✅ Generate bundle manifests
3. ✅ Create PR to k8s-operatorhub/community-operators (OperatorHub.io)
4. ✅ Create PR to redhat-openshift-ecosystem/community-operators-prod (OpenShift)

You'll get PRs automatically created in both catalogs!

### Manual Trigger

You can also manually trigger the submission workflow:

1. Go to https://github.com/mathianasj/virt-git-sync/actions/workflows/operator-catalog-pr.yml
2. Click "Run workflow"
3. Enter the version (e.g., `0.1.0` without the `v` prefix)
4. Click "Run workflow"

## Manual Submission Process (Alternative)

### 1. Test Your Bundle Locally

```bash
# Install operator-sdk if not already installed
brew install operator-sdk

# Validate bundle
operator-sdk bundle validate ./bundle

# Test with scorecard
operator-sdk scorecard ./bundle
```

### 2. Fork the Community Operators Repository

For **OpenShift** (Red Hat managed):
```bash
git clone https://github.com/redhat-openshift-ecosystem/community-operators-prod.git
cd community-operators-prod
git remote add upstream https://github.com/redhat-openshift-ecosystem/community-operators-prod.git
```

For **Kubernetes** (OperatorHub.io):
```bash
git clone https://github.com/k8s-operatorhub/community-operators.git
cd community-operators
git remote add upstream https://github.com/k8s-operatorhub/community-operators.git
```

### 3. Add Your Operator Bundle

The directory structure should be:
```
operators/
  virt-git-sync/
    0.1.0/              # Your version
      manifests/
        virt-git-sync.clusterserviceversion.yaml
        virt.mathianasj.github.com_virtgitsyncs.yaml
        ... (other manifests)
      metadata/
        annotations.yaml
      tests/
        ...
    ci.yaml             # CI configuration (optional)
```

Commands:
```bash
cd community-operators-prod  # or community-operators

# Create operator directory
mkdir -p operators/virt-git-sync/0.1.0

# Copy your bundle
cp -r /path/to/virt-git-sync/bundle/* operators/virt-git-sync/0.1.0/

# Create a branch
git checkout -b add-virt-git-sync-0.1.0
git add operators/virt-git-sync
git commit -m "Add virt-git-sync operator v0.1.0"
git push origin add-virt-git-sync-0.1.0
```

### 4. Create Pull Request

Open a PR on GitHub with:
- **Title**: `operator virt-git-sync (0.1.0)`
- **Description**: Brief description of your operator

### 5. CI Checks

The PR will trigger automated tests:
- Bundle validation
- Kubernetes deployment tests
- OpenShift deployment tests
- Security scans

Common issues:
- Container image must be publicly accessible
- CSV must have valid semver version
- All CRDs must be included
- RBAC permissions must be minimal

### 6. Review Process

- Bot will assign reviewers
- Address any feedback
- CI must be green
- Usually takes 1-2 weeks for review

### 7. Merge

Once approved and merged:
- **community-operators-prod**: Available in OpenShift OperatorHub within 24-48 hours
- **community-operators**: Available on OperatorHub.io immediately

## Before Submitting - Pre-flight Checklist

### Update CSV Metadata

- [ ] Add proper icon (base64 encoded, recommended 100x100px PNG)
- [ ] Update description with clear use cases
- [ ] Add keywords/categories
- [ ] Set proper maturity level (alpha/beta/stable)
- [ ] Add maintainers contact info
- [ ] Update links (docs, repository, support)

### Container Image

- [ ] Published to quay.io or other public registry
- [ ] Tagged with version (v0.1.0)
- [ ] Image is publicly pullable (no auth required)
- [ ] Multi-arch support recommended (amd64, arm64)

### Documentation

- [ ] README with operator overview
- [ ] Installation instructions
- [ ] Configuration examples
- [ ] Troubleshooting guide

### Security

- [ ] No hardcoded credentials
- [ ] Minimal RBAC permissions
- [ ] Container runs as non-root
- [ ] No privileged containers

### Testing

- [ ] Operator installs successfully
- [ ] CRD validation works
- [ ] Sample CR creates resources
- [ ] Operator handles errors gracefully
- [ ] Operator can be cleanly uninstalled

## Common Issues and Solutions

### Bundle Validation Failures

```bash
# Run locally to catch issues early
operator-sdk bundle validate ./bundle --select-optional suite=operatorframework

# Check for common issues
- CSV name must match: <package>.<version>
- Version must be semver
- Container image must exist and be public
- CRDs must be v1 (not v1beta1)
```

### CI Test Failures

- Check operator logs in CI
- Ensure RBAC has all required permissions
- Test in real cluster (Kind/Minikube) before submitting
- Use operator-sdk scorecard to catch issues

### Review Feedback

Common requests:
- Add icon (not placeholder)
- Improve description clarity
- Add more examples
- Update categories
- Fix RBAC over-permissions

## Resources

- Contribution guide: https://k8s-operatorhub.github.io/community-operators/contributing-via-pr/
- Packaging guide: https://sdk.operatorframework.io/docs/olm-integration/tutorial-bundle/
- Testing locally: https://sdk.operatorframework.io/docs/olm-integration/testing-deployment/
- Best practices: https://sdk.operatorframework.io/docs/best-practices/

## Commands Summary

```bash
# Test bundle locally
make bundle-build
operator-sdk bundle validate ./bundle
operator-sdk scorecard ./bundle

# Submit to OpenShift catalog
git clone https://github.com/redhat-openshift-ecosystem/community-operators-prod.git
cd community-operators-prod
mkdir -p operators/virt-git-sync/0.1.0
cp -r ../virt-git-sync/bundle/* operators/virt-git-sync/0.1.0/
git checkout -b add-virt-git-sync-0.1.0
git add operators/virt-git-sync
git commit -m "Add virt-git-sync operator v0.1.0"
git push origin add-virt-git-sync-0.1.0
# Then create PR on GitHub

# Submit to OperatorHub.io
git clone https://github.com/k8s-operatorhub/community-operators.git
cd community-operators
mkdir -p operators/virt-git-sync/0.1.0
cp -r ../virt-git-sync/bundle/* operators/virt-git-sync/0.1.0/
git checkout -b add-virt-git-sync-0.1.0
git add operators/virt-git-sync
git commit -m "Add virt-git-sync operator v0.1.0"
git push origin add-virt-git-sync-0.1.0
# Then create PR on GitHub
```
