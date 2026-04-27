#!/bin/bash

# Add all changes
git add -A

git commit -m "$(cat <<'EOF'
Add ArgoCD integration and auto-pause/unpause functionality

This commit includes the complete ArgoCD integration and auto-pause/unpause
feature that prevents race conditions between manual VM changes and ArgoCD sync.

ArgoCD Integration:
- Added ArgoCD Application CRD types and manager
- Disabled automated sync (operator manually triggers syncs)
- Added Repository Secret management for git credentials
- Added ignoreDifferences management for paused VMs
- Added manual sync triggering after git push

Auto-Pause/Unpause:
- Auto-pause VMs when manual changes detected (runStrategy, labels, etc.)
- Auto-unpause after syncing changes to git
- Prevents infinite loop between OpenShift UI and ArgoCD
- Comprehensive test coverage for auto-pause scenarios

Safety Mechanisms:
- Only sync when git working tree is clean
- Periodic reconciliation every 5 minutes
- Catches external git changes

Testing:
- Added auto-pause/unpause test suite
- Added GitHub Actions CI/CD workflow
- Added Makefile target for auto-pause tests

Documentation:
- Updated CLAUDE.md with ArgoCD integration details
- Added auto-pause testing instructions
- Added troubleshooting guide

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
EOF
)"

git push

rm -f /Users/mathianasj/git/virt-git-sync/commit-all.sh

echo "All changes committed and pushed successfully"
