# Dev Overlay

This overlay configures the operator for local development and testing.

## What it does

- Sets image tag to `dev` (mutable tag for testing)
- Sets `imagePullPolicy: Always` to avoid cached images
- Builds on top of `config/default`

## Usage

### Build manifests with dev config

```bash
kustomize build config/dev > install-dev.yaml
kubectl apply -f install-dev.yaml
```

### Build dev bundle

```bash
# Set dev image and generate bundle
make bundle IMG=quay.io/mathianasj/virt-git-sync:dev KUSTOMIZE_BUILD_DIR=config/dev
```

### Or use the automated script

```bash
./test-install-dev.sh
```

## Why separate from default?

- `config/default` = production config (versioned tags)
- `config/dev` = development config (mutable tags + always pull)
- Source control stays at production version
- Dev testing uses this overlay
