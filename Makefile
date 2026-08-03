.PHONY: generate test test-coverage test-registry-e2e test-admin-e2e test-boot-acceptance test-backup-restore-e2e build dev compose-up compose-up-postgres compose-down reset-local backup backup-inspect restore

DEV_ENV_FILE ?= .env

generate:
	cd backend && go tool oapi-codegen --config oapi-codegen.yaml api/openapi.yaml
	cd frontend && npm run generate:api

test:
	cd backend && go test ./...
	cd frontend && npm run lint && npm test && npm run typecheck

test-coverage:
	cd backend && mkdir -p coverage && go test -covermode=atomic -coverprofile=coverage/backend.out ./...
	cd frontend && npm run test:coverage

test-registry-e2e:
	cd backend && GROM_RUN_REGISTRY_E2E=1 go test -count=1 -timeout=10m ./tests/registrye2e

test-admin-e2e:
	cd frontend && npm run test:admin-e2e

test-boot-acceptance:
	cd backend && GROM_RUN_BOOT_ACCEPTANCE=1 go test -count=1 -timeout=12m ./tests/bootacceptance

test-backup-restore-e2e:
	cd backend && GROM_RUN_BACKUP_RESTORE_E2E=1 go test -count=1 -timeout=15m ./tests/backuprestoree2e

build:
	cd frontend && npm run build
	find backend/internal/webassets/dist -type f ! -name .keep -delete
	cp -R frontend/dist/. backend/internal/webassets/dist/
	cd backend && go build ./cmd/grom
	cd backend && go build ./cmd/grom-backup

dev:
	@if [ ! -f "$(DEV_ENV_FILE)" ]; then \
		echo "Missing $(DEV_ENV_FILE). Run: cp .env.example .env"; \
		exit 1; \
	fi
	@dev_env_file="$(DEV_ENV_FILE)"; \
	case "$$dev_env_file" in /*) ;; *) dev_env_file="./$$dev_env_file";; esac; \
	set -a; \
	. "$$dev_env_file"; \
	set +a; \
	cleanup() { \
		trap - INT TERM EXIT; \
		kill "$$backend_pid" "$$frontend_pid" 2>/dev/null || true; \
		wait "$$backend_pid" "$$frontend_pid" 2>/dev/null || true; \
	}; \
	handle_signal() { cleanup; exit 0; }; \
	trap handle_signal INT TERM; \
	trap cleanup EXIT; \
	(cd backend && go run ./cmd/grom) & backend_pid=$$!; \
	(cd frontend && npm run dev) & frontend_pid=$$!; \
	echo "Grom backend:  http://localhost:8080"; \
	echo "Grom frontend: http://localhost:5173"; \
	while kill -0 "$$backend_pid" 2>/dev/null && kill -0 "$$frontend_pid" 2>/dev/null; do \
		sleep 1; \
	done; \
	status=0; \
	if ! kill -0 "$$backend_pid" 2>/dev/null; then wait "$$backend_pid" || status=$$?; fi; \
	if ! kill -0 "$$frontend_pid" 2>/dev/null; then wait "$$frontend_pid" || status=$$?; fi; \
	exit "$$status"

compose-up:
	docker compose --env-file .env -f deploy/compose/docker-compose.yml up --build

compose-up-postgres:
	docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.postgres.yml up --build

compose-down:
	docker compose --env-file .env -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.postgres.yml down

reset-local:
	@if [ ! -f "$(DEV_ENV_FILE)" ]; then \
		echo "Missing $(DEV_ENV_FILE). Run: cp .env.example .env"; \
		exit 1; \
	fi
	@echo "Resetting the local Grom stack and deleting its development data..."
	docker compose --env-file "$(DEV_ENV_FILE)" -f deploy/compose/docker-compose.yml -f deploy/compose/docker-compose.postgres.yml down --volumes --remove-orphans
	rm -rf -- "$(CURDIR)/backend/data" "$(CURDIR)/data"
	@echo "Local reset complete. Docker volumes and local development data were removed."

backup:
	@test -n "$(BACKUP_DIR)" || (echo "BACKUP_DIR must be an explicit absolute host path" >&2; exit 1)
	@deploy/backup/backup-compose.sh create "$(BACKUP_DIR)"

backup-inspect:
	@test -n "$(BACKUP_PATH)" || (echo "BACKUP_PATH must be an explicit absolute backup path" >&2; exit 1)
	@deploy/backup/backup-compose.sh inspect "$(BACKUP_PATH)"

restore:
	@test -n "$(BACKUP_PATH)" || (echo "BACKUP_PATH must be an explicit absolute backup path" >&2; exit 1)
	@deploy/backup/restore-compose.sh "$(BACKUP_PATH)"
