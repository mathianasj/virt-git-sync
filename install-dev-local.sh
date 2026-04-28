#!/bin/bash
set -e

# Install virt-git-sync operator locally (without OLM)
# Uses kustomize dev overlay for dev tag and imagePullPolicy: Always
# This is faster for development but doesn't test OLM integration

DEV_TAG=${DEV_TAG:-dev}
OPERATOR_IMG="quay.io/mathianasj/virt-git-sync:${DEV_TAG}"

echo "======================================"
echo "VirtGitSync Operator - Local Install (no OLM)"
echo "======================================"
echo ""
echo "Operator Image: $OPERATOR_IMG"
echo "Platform: linux/amd64 (for OpenShift x86_64 clusters)"
echo ""

# Step 1: Build operator image for amd64
echo "Step 1: Build operator image for amd64"
echo ""
echo "NOTE: Building for linux/amd64 to run on OpenShift x86_64 nodes"
echo ""
podman build --platform linux/amd64 -t $OPERATOR_IMG -f Dockerfile .

echo ""
echo "✓ Operator image built: $OPERATOR_IMG"
echo ""

# Step 2: Push image
echo "Step 2: Push image to quay.io"
echo ""
echo "Logging into quay.io..."
podman login quay.io

echo ""
echo "Pushing operator image..."
podman push $OPERATOR_IMG

echo ""
echo "✓ Operator image pushed: $OPERATOR_IMG"
echo ""

# Step 3: Make image public
echo "======================================"
echo "IMPORTANT: Make repository public"
echo "======================================"
echo ""
echo "Make sure the repository is public on quay.io:"
echo "https://quay.io/repository/mathianasj/virt-git-sync?tab=settings"
echo ""
read -p "Press enter when repository is public..."
echo ""

# Step 4: Install CRDs
echo "Step 4: Install CRDs"
echo ""
make install

echo ""
echo "✓ CRDs installed"
echo ""

# Step 5: Deploy operator using dev overlay
echo "Step 5: Deploy operator using dev kustomize overlay"
echo ""
echo "Using: kustomize build config/dev"
echo ""

kubectl apply -k config/dev

echo ""
echo "✓ Operator deployed"
echo ""

echo "======================================"
echo "Installation Complete!"
echo "======================================"
echo ""
echo "The operator is running with:"
echo "  - Image: $OPERATOR_IMG"
echo "  - imagePullPolicy: Always (via config/dev overlay)"
echo ""
echo "Check deployment status:"
echo "  kubectl get deployment -n virt-git-sync-system"
echo "  kubectl get pods -n virt-git-sync-system"
echo ""
echo "View operator logs:"
echo "  kubectl logs -n virt-git-sync-system -l control-plane=controller-manager -f"
echo ""
echo "Create a test VirtGitSync CR:"
echo "  kubectl apply -f config/samples/virt_v1alpha1_virtgitsync.yaml"
echo ""
echo "Watch VirtGitSync status:"
echo "  kubectl get virtgitsync -n default -o wide -w"
echo ""
echo "Cleanup when done:"
echo "  kubectl delete -k config/dev"
echo "  make uninstall"
echo ""
