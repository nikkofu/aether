#!/bin/bash
set -e

mkdir -p internal/core
mkdir -p internal/adapters
mkdir -p internal/pkg
mkdir -p internal/ports

# Order matters for capabilities vs capability
mv internal/capabilities internal/adapters/capabilities
mv internal/capability internal/core/capability
mv internal/cli_adapters internal/adapters/llm

# Core
for pkg in agent org dag strategy learning knowledge reflection economy governance policy risk issue skills memory system cluster session; do
    if [ -d "internal/$pkg" ]; then
        mv "internal/$pkg" "internal/core/$pkg"
    fi
done

# Pkg
for pkg in config bus logging audit metrics observability routing security; do
    if [ -d "internal/$pkg" ]; then
        mv "internal/$pkg" "internal/pkg/$pkg"
    fi
done

# Ports
for pkg in cli todo; do
    if [ -d "internal/$pkg" ]; then
        mv "internal/$pkg" "internal/ports/$pkg"
    fi
done

echo "Directories moved."

# Update imports in all go files
# Capabilities and capability first
find . -name "*.go" -exec sed -i '' 's|"github.com/nikkofu/aether/internal/capabilities|"github.com/nikkofu/aether/internal/adapters/capabilities|g' {} +
find . -name "*.go" -exec sed -i '' 's|"github.com/nikkofu/aether/internal/capability|"github.com/nikkofu/aether/internal/core/capability|g' {} +
find . -name "*.go" -exec sed -i '' 's|"github.com/nikkofu/aether/internal/cli_adapters|"github.com/nikkofu/aether/internal/adapters/llm|g' {} +

# Core
for pkg in agent org dag strategy learning knowledge reflection economy governance policy risk issue skills memory system cluster session; do
    find . -name "*.go" -exec sed -i '' "s|\"github.com/nikkofu/aether/internal/$pkg|\"github.com/nikkofu/aether/internal/core/$pkg|g" {} +
done

# Pkg
for pkg in config bus logging audit metrics observability routing security; do
    find . -name "*.go" -exec sed -i '' "s|\"github.com/nikkofu/aether/internal/$pkg|\"github.com/nikkofu/aether/internal/pkg/$pkg|g" {} +
done

# Ports
for pkg in cli todo; do
    find . -name "*.go" -exec sed -i '' "s|\"github.com/nikkofu/aether/internal/$pkg|\"github.com/nikkofu/aether/internal/ports/$pkg|g" {} +
done

echo "Imports updated."
