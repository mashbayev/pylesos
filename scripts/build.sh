#!/usr/bin/env bash
# Файл: scripts/build.sh
# Полная сборка (CLAUDE.md, правило 9): фронт → проверки Go → бинарник.
set -euo pipefail
cd "$(dirname "$0")/.."
(cd web && npm ci && npm run typecheck && npm run build)
touch web/dist/.keep
go vet ./...
go test ./...
go build -o bin/pylesos ./cmd/pylesos
echo "Готово: bin/pylesos"
