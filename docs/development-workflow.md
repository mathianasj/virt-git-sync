# Development Workflow

## Source Control Strategy

### What's in Git

- **Production configuration** - `config/manager/kustomization.yaml` uses the latest release tag (v0.1.0)
- **Production bundle** - `bundle/` contains manifests for the latest release
- **Dev overlay** - `config/dev/` contains kustomize overlay for development testing

### What's Generated

- Bundle manifests are **regenerated at release time** by GitHub Actions
- Dev testing **temporarily modifies** files (don't commit these changes)

## Development Approaches

### Option 1: Local Development (Fastest)

Run the operator on your local machine - no container images needed:

```bash
# Install CRDs
make install

# Run operator locally
make run
```

**Pros:** Instant iteration, no image builds  
**Cons:** Doesn't test containerization or multi-arch

### Option 2: Local Cluster Install (No OLM)

Deploy to cluster without OLM - uses kustomize dev overlay:

```bash
./install-dev-local.sh
```

This:
1. Builds amd64 image
2. Pushes to quay.io
3. Deploys using `kubectl apply -k config/dev`

**Pros:** Tests containerization, fast feedback  
**Cons:** Doesn't test OLM installation

### Option 3: OLM Install (Full Integration Test)

Test the full OLM installation workflow:

```bash
./test-install-dev.sh
```

This:
1. Builds amd64 operator image
2. Generates bundle (temporarily modifies files)
3. Builds bundle image
4. Installs via `operator-sdk run bundle`

**Pros:** Tests complete user experience  
**Cons:** Slower, modifies source files

**Important:** After OLM testing, restore production config:
```bash
git restore bundle/ config/manager/kustomization.yaml
```

## Architecture Requirements

### Development Machine vs Target Cluster

| Platform | Architecture | Notes |
|----------|--------------|-------|
| Apple Silicon Mac | arm64 | Your build machine |
| OpenShift/Linux servers | amd64/x86_64 | Target cluster |

**Always build for amd64** when deploying to OpenShift:

```bash
# Manual build
podman build --platform linux/amd64 -t image:tag .

# Or use Makefile
make docker-build-amd64 IMG=image:tag
```

### Image Pull Policy

| Tag Type | Pull Policy | Use Case |
|----------|-------------|----------|
| `v0.1.0` | `IfNotPresent` | Production - immutable tags |
| `:latest` | `Always` | Auto-set by Kubernetes |
| `:dev` | `Always` | Dev - mutable tag needs this |

The **dev overlay** automatically sets `imagePullPolicy: Always`.

## Release Workflow

### Creating a Release

1. **Ensure tests pass:**
   ```bash
   make test
   ```

2. **Create and push tag:**
   ```bash
   git tag v0.2.0
   git push origin v0.2.0
   ```

3. **GitHub Actions automatically:**
   - Builds multi-arch image (amd64 + arm64)
   - Generates bundle with versioned image
   - Creates GitHub release with artifacts
   - Pushes bundle and catalog images

### What Happens

The release workflow (`.github/workflows/release.yml`):

1. **Builds multi-arch operator image:**
   ```yaml
   platforms: linux/amd64,linux/arm64
   ```

2. **Generates bundle:**
   ```bash
   make bundle IMG=quay.io/mathianasj/virt-git-sync:v0.2.0
   ```

3. **Creates artifacts:**
   - Operator: `quay.io/mathianasj/virt-git-sync:v0.2.0`
   - Bundle: `quay.io/mathianasj/virt-git-sync-bundle:v0.2.0`
   - Catalog: `quay.io/mathianasj/virt-git-sync-catalog:v0.2.0`

## File Modification Policy

### Never Commit

- Bundle manifests with `:dev` tag
- `config/manager/kustomization.yaml` with `newTag: dev`
- Temporary build artifacts

### Always Commit

- Bundle manifests with release tags (via GitHub Actions)
- `config/manager/kustomization.yaml` with `newTag: v0.x.y`
- Source code changes
- Test updates
- Documentation

### Reset After Dev Testing

```bash
# Restore production configuration
git restore bundle/ config/manager/kustomization.yaml

# Or reset everything
git reset --hard origin/master
```

## Kustomize Overlays

### Structure

```
config/
├── default/          # Base production config
│   └── kustomization.yaml
├── dev/              # Development overlay
│   ├── kustomization.yaml
│   └── README.md
├── manager/
│   ├── kustomization.yaml  # Image tag set here
│   └── manager.yaml
└── ...
```

### Using Overlays

**Production:**
```bash
kubectl apply -k config/default
```

**Development:**
```bash
kubectl apply -k config/dev
```

**Custom:**
```bash
kustomize build config/dev | kubectl apply -f -
```

## Common Tasks

### Update CRD

1. Modify `api/v1alpha1/*_types.go`
2. Regenerate:
   ```bash
   make generate
   make manifests
   ```
3. Update CRD in cluster:
   ```bash
   make install
   ```

### Update RBAC

1. Add kubebuilder markers in controller
2. Regenerate:
   ```bash
   make manifests
   ```

### Run Tests

```bash
# All tests
make test

# Specific package
go test ./internal/controller/... -v

# With coverage
make test
go tool cover -html=cover.out
```

### Build Multi-Arch for Production

```bash
make docker-buildx IMG=quay.io/mathianasj/virt-git-sync:v0.2.0
```

This builds for multiple platforms and pushes to registry.

## Troubleshooting

### "exec format error" in pods

**Cause:** Image architecture doesn't match cluster  
**Fix:** Rebuild for amd64: `make docker-build-amd64 IMG=...`

### Bundle has dev tag after testing

**Cause:** `make bundle` modified files  
**Fix:** `git restore bundle/ config/manager/kustomization.yaml`

### Image pull failure

**Cause:** Repository not public on quay.io  
**Fix:** Set repository to public in quay.io settings

### OLM installation stuck

**Check:** `kubectl get installplan -A`  
**Logs:** `kubectl logs -n olm pod-name`  
**Reset:** `operator-sdk cleanup virt-git-sync`

## Best Practices

1. **Use local dev (`make run`) for iteration** - fastest feedback
2. **Test with local install before OLM** - catches container issues
3. **Always restore after OLM testing** - keep source clean
4. **Build for target architecture** - amd64 for OpenShift
5. **Use dev overlay for cluster testing** - automatic imagePullPolicy

## References

- [Kustomize documentation](https://kustomize.io/)
- [OLM documentation](https://olm.operatorframework.io/)
- [Operator SDK documentation](https://sdk.operatorframework.io/)
- [Building for OpenShift](./building-for-openshift.md)
