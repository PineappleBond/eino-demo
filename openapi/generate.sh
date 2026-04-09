#!/usr/bin/env bash
# Generate types for Go and TypeScript from openapi/spec.yaml.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"
SPEC="$SCRIPT_DIR/spec.yaml"

if [ ! -f "$SPEC" ]; then
  echo "error: spec.yaml not found at $SPEC" >&2
  exit 1
fi

echo "Generating Go types..."
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest 2>/dev/null
oapi-codegen -generate types,skip-prune -package types "$SPEC" > "$ROOT_DIR/server/internal/types/types.go"
echo "  → server/internal/types/types.go"

echo "Generating TypeScript types..."
npx --yes openapi-typescript "$SPEC" -o "$ROOT_DIR/web/src/types/api.d.ts"
echo "  → web/src/types/api.d.ts"

echo "Done."
