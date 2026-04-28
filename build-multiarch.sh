#!/bin/bash
set -e

# Build multi-architecture operator images
# Builds for amd64 to work with OpenShift/x86_64 clusters

VERSION=${VERSION:-v0.1.0}
DEV_TAG=${DEV_TAG:-dev}
REGISTRY=${REGISTRY:-quay.io/mathianasj}
OPERATOR_NAME=virt-git-sync

# Determine which tag to use
if [ "$USE_DEV" = "true" ]; then
    TAG=$DEV_TAG
else
    TAG=$VERSION
fi

OPERATOR_IMG="${REGISTRY}/${OPERATOR_NAME}:${TAG}"
BUNDLE_IMG="${REGISTRY}/${OPERATOR_NAME}-bundle:${TAG}"

echo "======================================"
echo "Building Multi-Architecture Images"
echo "======================================"
echo ""
echo "Operator Image: $OPERATOR_IMG"
echo "Bundle Image: $BUNDLE_IMG"
echo "Platform: linux/amd64"
echo ""

# Step 1: Build operator image for amd64
echo "Step 1: Build operator image for amd64"
echo ""
podman build --platform linux/amd64 -t $OPERATOR_IMG -f Dockerfile .

echo ""
echo "✓ Operator image built: $OPERATOR_IMG"
echo ""

# Step 2: Generate bundle manifests
echo "Step 2: Generate bundle manifests"
echo ""
make bundle IMG=$OPERATOR_IMG

echo ""
echo "✓ Bundle manifests generated"
echo ""

# Step 3: Ensure imagePullPolicy is Always for dev builds
if [ "$USE_DEV" = "true" ]; then
    echo "Step 3: Setting imagePullPolicy to Always for dev build"
    echo ""

    # Check if imagePullPolicy is already set
    if ! grep -q "imagePullPolicy: Always" bundle/manifests/virt-git-sync.clusterserviceversion.yaml; then
        # Use sed to add imagePullPolicy after the image line
        sed -i.bak '/image: quay.io\/mathianasj\/virt-git-sync/a\
                imagePullPolicy: Always' bundle/manifests/virt-git-sync.clusterserviceversion.yaml
        rm bundle/manifests/virt-git-sync.clusterserviceversion.yaml.bak
        echo "✓ Added imagePullPolicy: Always"
    else
        echo "✓ imagePullPolicy: Always already set"
    fi
    echo ""
fi

# Step 4: Build bundle image
echo "Step 4: Build bundle image"
echo ""
podman build -f bundle.Dockerfile -t $BUNDLE_IMG .

echo ""
echo "✓ Bundle image built: $BUNDLE_IMG"
echo ""

# Step 5: Push images
read -p "Push images to registry? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo ""
    echo "Step 5: Pushing images to registry"
    echo ""

    echo "Logging into $REGISTRY..."
    podman login $REGISTRY

    echo ""
    echo "Pushing operator image..."
    podman push $OPERATOR_IMG

    echo ""
    echo "Pushing bundle image..."
    podman push $BUNDLE_IMG

    echo ""
    echo "✓ Images pushed successfully"
    echo ""
else
    echo ""
    echo "Skipped pushing images"
    echo ""
fi

echo "======================================"
echo "Build Complete!"
echo "======================================"
echo ""
echo "Images built:"
echo "  Operator: $OPERATOR_IMG"
echo "  Bundle:   $BUNDLE_IMG"
echo ""
echo "Next steps:"
echo "  1. Make repositories public on quay.io (if needed)"
echo "  2. Install via OLM:"
echo "     operator-sdk run bundle $BUNDLE_IMG"
echo ""
