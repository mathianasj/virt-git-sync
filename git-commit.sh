#!/bin/bash
# Git commit script that prompts for GPG password
# Usage: ./git-commit.sh "Your commit message"

set -e

if [ -z "$1" ]; then
    echo "Error: Commit message required"
    echo "Usage: ./git-commit.sh \"Your commit message\""
    exit 1
fi

COMMIT_MSG="$1"

echo "======================================"
echo "Git Commit with GPG Signing"
echo "======================================"
echo ""
echo "Commit message:"
echo "$COMMIT_MSG"
echo ""
echo "You will be prompted for your GPG password..."
echo ""

# Use --no-gpg-sign to skip GPG or remove this flag to enable GPG
# The GPG password prompt will appear in a GUI dialog
# --signoff adds "Signed-off-by: <email>" (required for DCO)
git commit --signoff -m "$COMMIT_MSG"

echo ""
echo "✓ Commit successful!"
echo ""
git log -1 --oneline
