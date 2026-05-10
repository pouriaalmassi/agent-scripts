#!/bin/bash

set -e

echo "Checking for GitHub CLI (gh)..."

# Check if gh CLI is installed
if ! command -v gh &> /dev/null; then
    echo "gh CLI not found. Detecting OS..."
    OS="$(uname -s)"
    
    case "${OS}" in
        Darwin*)
            echo "macOS detected. Attempting to install via Homebrew..."
            if ! command -v brew &> /dev/null; then
                echo "Error: Homebrew not found. Please install Homebrew (https://brew.sh/) or gh CLI manually."
                exit 1
            fi
            brew install gh
            ;;
        Linux*)
            echo "Linux detected. Please install gh CLI using your package manager (e.g., sudo apt install gh)."
            echo "See: https://github.com/cli/cli/blob/trunk/INSTALLATION.md"
            exit 1
            ;;
        *)
            echo "Unsupported OS: ${OS}. Please install gh CLI manually: https://cli.github.com/"
            exit 1
            ;;
    esac
else
    echo "gh CLI is already installed."
fi

# Install or upgrade the gh-pr-review extension
echo "Checking for gh-pr-review extension..."
if gh extension list | grep -q "gh-pr-review"; then
    echo "Extension is already installed. Upgrading..."
    gh extension upgrade gh-pr-review || true
else
    echo "Installing agynio/gh-pr-review extension..."
    gh extension install agynio/gh-pr-review
fi

echo "------------------------------------------------"
echo "Setup complete! You can now use the skill."
echo "Try: gh pr-review --help"
echo "------------------------------------------------"
