#!/bin/bash
set -e

# Test install virt-git-sync operator using operator-sdk with dev tag
# This uses OLM to install the operator from a dev bundle image
# IMPORTANT: Builds for linux/amd64 to work with OpenShift/x86_64 clusters
#
# NOTE: This script temporarily modifies bundle manifests. Reset with:
#       git restore bundle/ config/manager/kustomization.yaml

DEV_TAG=${DEV_TAG:-dev}
OPERATOR_IMG="quay.io/mathianasj/virt-git-sync:${DEV_TAG}"
BUNDLE_IMG="quay.io/mathianasj/virt-git-sync-bundle:${DEV_TAG}"

echo "======================================"
echo "VirtGitSync Operator - Dev Install via OLM"
echo "======================================"
echo ""
echo "Operator Image: $OPERATOR_IMG"
echo "Bundle Image: $BUNDLE_IMG"
echo "Platform: linux/amd64 (for OpenShift x86_64 clusters)"
echo ""

# Step 1: Build images for amd64 architecture
echo "Step 1: Build operator image for amd64"
echo ""
echo "NOTE: Building for linux/amd64 to run on OpenShift x86_64 nodes"
echo ""
podman build --platform linux/amd64 -t $OPERATOR_IMG -f Dockerfile .

echo ""
echo "✓ Operator image built: $OPERATOR_IMG"
echo ""

# Step 2: Regenerate bundle with dev image
echo "Step 2: Generate bundle manifests with dev image"
echo ""
echo "NOTE: This temporarily modifies config/manager/kustomization.yaml and bundle/"
echo ""
make bundle IMG=$OPERATOR_IMG

echo ""
echo "✓ Bundle manifests generated"
echo ""

# Step 3: Ensure imagePullPolicy is Always for dev builds
echo "Step 3: Setting imagePullPolicy to Always"
echo ""
echo "NOTE: Using 'Always' to ensure fresh image pull with mutable dev tag"
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

# Step 4: Build bundle image
echo "Step 4: Build bundle image"
echo ""
podman build -f bundle.Dockerfile -t $BUNDLE_IMG .

echo ""
echo "✓ Bundle image built: $BUNDLE_IMG"
echo ""

# Step 5: Login and push to quay.io
echo "Step 5: Push images to quay.io"
echo ""
echo "Logging into quay.io..."
podman login quay.io

echo ""
echo "Pushing operator image..."
podman push $OPERATOR_IMG

echo ""
echo "Pushing bundle image..."
podman push $BUNDLE_IMG

echo ""
echo "✓ Images pushed to quay.io"
echo ""

# Step 6: Make images public
echo "======================================"
echo "IMPORTANT: Make repositories public"
echo "======================================"
echo ""
echo "Before installing, make sure these repositories are public on quay.io:"
echo ""
echo "1. Operator repo: https://quay.io/repository/mathianasj/virt-git-sync?tab=settings"
echo "2. Bundle repo:   https://quay.io/repository/mathianasj/virt-git-sync-bundle?tab=settings"
echo ""
read -p "Press enter when repositories are public..."
echo ""

# Step 7: Install via OLM
echo "Step 7: Install operator via OLM"
echo ""
echo "Running: operator-sdk run bundle $BUNDLE_IMG"
echo ""

operator-sdk run bundle $BUNDLE_IMG

echo ""
echo "======================================"
echo "Installation Complete!"
echo "======================================"
echo ""
echo "Check installation status:"
echo "  kubectl get csv -A | grep virt-git-sync"
echo "  kubectl get pods -A | grep virt-git-sync"
echo ""
echo "View operator logs:"
echo "  kubectl logs -n default -l control-plane=controller-manager -f"
echo ""
echo "Create a test VirtGitSync CR:"
echo "  kubectl apply -f config/samples/virt_v1alpha1_virtgitsync.yaml"
echo ""
echo "Watch VirtGitSync status:"
echo "  kubectl get virtgitsync -o wide -w"
echo ""
echo "Cleanup when done:"
echo "  operator-sdk cleanup virt-git-sync"
echo ""
echo "Reset modified files:"
echo "  git restore bundle/ config/manager/kustomization.yaml"
echo ""
