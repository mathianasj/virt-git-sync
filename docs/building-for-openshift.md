# Building for OpenShift

## Architecture Requirements

OpenShift clusters typically run on **x86_64/amd64** architecture, while development on Apple Silicon Macs uses **arm64**. 

**Important:** You must build operator images for the target cluster architecture, not your local machine architecture.

## Quick Start

### For Development/Testing on OpenShift

Use the automated test installation script:

```bash
./test-install-dev.sh
```

This script:
1. Builds operator image for **linux/amd64** architecture
2. Generates bundle manifests with dev tag
3. Sets `imagePullPolicy: Always` to avoid caching issues
4. Builds and pushes bundle image
5. Prompts for repository visibility confirmation
6. Installs operator via OLM

### Using the Makefile

Build for amd64 architecture:

```bash
make docker-build-amd64 IMG=quay.io/mathianasj/virt-git-sync:dev
```

Then push:

```bash
make docker-push IMG=quay.io/mathianasj/virt-git-sync:dev
```

## Common Issues

### Issue: "exec container process `/manager`: Exec format error"

**Cause:** Image architecture doesn't match cluster architecture (e.g., arm64 image on amd64 cluster)

**Solution:**
1. Rebuild for correct architecture:
   ```bash
   make docker-build-amd64 IMG=quay.io/mathianasj/virt-git-sync:dev
   make docker-push IMG=quay.io/mathianasj/virt-git-sync:dev
   ```

2. If using dev tag, ensure `imagePullPolicy: Always` in CSV to avoid cached images:
   ```yaml
   # bundle/manifests/virt-git-sync.clusterserviceversion.yaml
   spec:
     install:
       spec:
         deployments:
           - spec:
               template:
                 spec:
                   containers:
                     - image: quay.io/mathianasj/virt-git-sync:dev
                       imagePullPolicy: Always  # <-- Important for dev builds
   ```

3. Clean up and reinstall:
   ```bash
   operator-sdk cleanup virt-git-sync
   operator-sdk run bundle quay.io/mathianasj/virt-git-sync-bundle:dev
   ```

### Issue: Pod pulls cached image despite new build

**Cause:** Using `imagePullPolicy: IfNotPresent` (default) with reused tags like `dev`

**Solutions:**
1. Use `imagePullPolicy: Always` in CSV (recommended for dev)
2. Use unique tags with timestamps (e.g., `dev-1714234567`)
3. Manually delete cached images on cluster nodes

## Production Builds

For production releases, GitHub Actions automatically builds for amd64:

```bash
git tag -a v0.2.0 -m "Release v0.2.0"
git push origin v0.2.0
```

The release workflow builds and pushes the amd64 image to quay.io.

## Verification

Check image architecture:

```bash
# Locally
podman inspect quay.io/mathianasj/virt-git-sync:dev | jq -r '.[0].Architecture'

# From registry
podman pull quay.io/mathianasj/virt-git-sync:dev
podman inspect quay.io/mathianasj/virt-git-sync:dev | jq -r '.[0].Architecture'
```

Check cluster node architecture:

```bash
kubectl get nodes -o wide
# Look at KERNEL-VERSION column - x86_64 or aarch64
```

## Development Workflow

Recommended workflow for Apple Silicon Mac → OpenShift development:

1. **Local development:** Run operator locally
   ```bash
   make install  # Install CRDs
   make run      # Run locally (no architecture issues)
   ```

2. **Testing on cluster:** Build for amd64
   ```bash
   ./test-install-dev.sh
   ```

3. **Production release:** Push version tag
   ```bash
   git tag -a v0.2.0 -m "Release v0.2.0"
   git push origin v0.2.0
   ```

## References

- [Podman platform builds](https://docs.podman.io/en/latest/markdown/podman-build.1.html#platform)
- [Kubernetes image pull policy](https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy)
- [OLM architecture support](https://olm.operatorframework.io/docs/best-practices/common/#supporting-multiple-architectures-and-operating-systems)
