#!/bin/bash
set -e

# VirtGitSync Release Script
# Automates the complete release process including:
# 1. Version validation and testing
# 2. Updating Makefile and bundle manifests
# 3. Committing version changes
# 4. Creating and pushing git tag
# 5. Triggering GitHub Actions release workflow

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "======================================"
echo "VirtGitSync Operator - Release"
echo "======================================"
echo ""

# Function to print colored output
print_step() {
    echo -e "${BLUE}==>${NC} $1"
}

print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# Function to prompt for confirmation
confirm() {
    read -p "$1 (y/n) " -n 1 -r
    echo
    [[ $REPLY =~ ^[Yy]$ ]]
}

# Check we're on master branch
print_step "Checking git branch..."
CURRENT_BRANCH=$(git branch --show-current)
if [ "$CURRENT_BRANCH" != "master" ]; then
    print_error "Must be on master branch (currently on: $CURRENT_BRANCH)"
    if confirm "Switch to master?"; then
        git checkout master
        print_success "Switched to master"
    else
        exit 1
    fi
fi

# Check for uncommitted changes
print_step "Checking for uncommitted changes..."
if ! git diff-index --quiet HEAD --; then
    print_error "Uncommitted changes detected"
    git status --short
    echo ""
    if confirm "Commit these changes first?"; then
        read -p "Enter commit message: " COMMIT_MSG
        ./git-commit.sh "$COMMIT_MSG"
        print_success "Changes committed"
    else
        print_error "Please commit or stash changes before releasing"
        exit 1
    fi
fi

# Pull latest changes
print_step "Pulling latest changes from origin..."
if git pull origin master --ff-only; then
    print_success "Up to date with origin/master"
else
    print_error "Failed to pull from origin. Resolve conflicts and try again."
    exit 1
fi

# Get current version from Makefile
CURRENT_VERSION=$(grep "^VERSION ?=" Makefile | awk '{print $3}')
print_step "Current version: $CURRENT_VERSION"

# Prompt for new version
echo ""
read -p "Enter new version (e.g., 0.2.0 without 'v' prefix): " NEW_VERSION

# Validate version format (semantic versioning)
if ! [[ $NEW_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    print_error "Invalid version format. Must be semantic version (e.g., 0.2.0)"
    exit 1
fi

VERSION_TAG="v${NEW_VERSION}"

# Check if tag already exists
if git rev-parse "$VERSION_TAG" >/dev/null 2>&1; then
    print_error "Tag $VERSION_TAG already exists"
    exit 1
fi

print_success "Version validated: $VERSION_TAG"
echo ""

# Run tests
print_step "Running tests..."
if make test; then
    print_success "All tests passed"
else
    print_error "Tests failed"
    if ! confirm "Continue anyway?"; then
        exit 1
    fi
fi
echo ""

# Validate bundle
print_step "Validating bundle..."
if operator-sdk bundle validate ./bundle; then
    print_success "Bundle validation passed"
else
    print_error "Bundle validation failed"
    if ! confirm "Continue anyway?"; then
        exit 1
    fi
fi
echo ""

# Update version files
print_step "Updating version files..."

# Update Makefile VERSION
sed -i.bak "s/^VERSION ?= .*/VERSION ?= $NEW_VERSION/" Makefile
rm -f Makefile.bak
print_success "Updated Makefile VERSION to $NEW_VERSION"

# Regenerate bundle with new version
print_step "Regenerating bundle with version $NEW_VERSION..."
IMAGE_TAG="quay.io/mathianasj/virt-git-sync:v${NEW_VERSION}"
if make bundle VERSION="$NEW_VERSION" IMG="$IMAGE_TAG"; then
    print_success "Bundle regenerated with image $IMAGE_TAG"
else
    print_error "Bundle generation failed"
    exit 1
fi

# Validate regenerated bundle
print_step "Validating regenerated bundle..."
if operator-sdk bundle validate ./bundle; then
    print_success "Regenerated bundle validated"
else
    print_error "Regenerated bundle validation failed"
    exit 1
fi

# Commit version changes
print_step "Committing version changes..."
git add Makefile bundle/
COMMIT_MSG="Release $VERSION_TAG - Update bundle and manifests

- Update Makefile VERSION to $NEW_VERSION
- Regenerate bundle manifests with correct version
- Update CSV to version $NEW_VERSION
- Update container image tags to $VERSION_TAG"

if ./git-commit.sh "$COMMIT_MSG"; then
    print_success "Version changes committed"
else
    print_error "Failed to commit version changes"
    exit 1
fi
echo ""

# Summary
echo "======================================"
echo "Release Summary"
echo "======================================"
echo "  Version:        $VERSION_TAG"
echo "  Current branch: $CURRENT_BRANCH"
echo "  Tests:          Passed"
echo "  Bundle:         Updated & Committed"
echo ""
echo "Version changes committed:"
echo "  ✓ Makefile VERSION updated to $NEW_VERSION"
echo "  ✓ Bundle manifests regenerated"
echo "  ✓ CSV version updated to $NEW_VERSION"
echo "  ✓ Container image tags updated to $VERSION_TAG"
echo ""
echo "This will trigger GitHub Actions to:"
echo "  1. Build multi-arch images (amd64, arm64)"
echo "  2. Build and publish OLM bundle"
echo "  3. Create GitHub release with artifacts"
echo "  4. Create PRs to OperatorHub.io catalog"
echo "  5. Create PRs to OpenShift catalog"
echo ""
echo "======================================"
echo ""

if ! confirm "Create and push release tag $VERSION_TAG?"; then
    print_warning "Release cancelled"
    print_warning "Note: Version changes have been committed to git"
    print_warning "You may want to revert the commit if cancelling the release"
    exit 0
fi

# Create and push tag
print_step "Creating tag $VERSION_TAG..."
git tag -a "$VERSION_TAG" -m "Release $VERSION_TAG"
print_success "Tag created"

print_step "Pushing tag to origin..."
if git push origin "$VERSION_TAG"; then
    print_success "Tag pushed to origin"
else
    print_error "Failed to push tag"
    print_warning "You can manually push with: git push origin $VERSION_TAG"
    exit 1
fi

# Also push any commits on master
print_step "Pushing master branch..."
if git push origin master; then
    print_success "Master branch pushed"
else
    print_warning "Master branch push failed (may already be up to date)"
fi

echo ""
echo "======================================"
echo "Release Complete! 🎉"
echo "======================================"
echo ""
echo "Tag $VERSION_TAG has been created and pushed."
echo ""
echo "GitHub Actions is now:"
echo "  • Building multi-arch images"
echo "  • Creating GitHub release"
echo "  • Generating operator catalog PRs"
echo ""
echo "Monitor progress:"
echo "  Release workflow:  https://github.com/mathianasj/virt-git-sync/actions/workflows/release.yml"
echo "  Catalog PRs:       https://github.com/mathianasj/virt-git-sync/actions/workflows/operator-catalog-pr.yml"
echo ""
echo "Images will be available at:"
echo "  Operator:          quay.io/mathianasj/virt-git-sync:$VERSION_TAG"
echo "  Bundle:            quay.io/mathianasj/virt-git-sync-bundle:$VERSION_TAG"
echo "  Catalog:           quay.io/mathianasj/virt-git-sync-catalog:$VERSION_TAG"
echo ""
echo "GitHub Release:      https://github.com/mathianasj/virt-git-sync/releases/tag/$VERSION_TAG"
echo ""
echo "Next steps:"
echo "  1. Wait for GitHub Actions to complete (~10 minutes)"
echo "  2. Review the created GitHub release"
echo "  3. Check PRs in your forks:"
echo "     - Fork of k8s-operatorhub/community-operators"
echo "     - Fork of redhat-openshift-ecosystem/community-operators-prod"
echo "  4. PRs will be auto-created to upstream catalogs"
echo "  5. Monitor and respond to catalog PR reviews"
echo ""
