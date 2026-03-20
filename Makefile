.PHONY: mnemonic help run stop

default: help

mnemonic: ## Run the build.sh script to build the mnemonic server
	@printf "Starting full build of mnemonicn...\n"
	@LOCAL=1 ./src/mnemonic/build/build.sh

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nAvailable targets:\n"} /^[a-zA-Z0-9_-]+:.*##/ { printf "  %-12s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

start: ## Runs mnemonic using the latest build in the ghcr
	@printf "Starting mnemonic..."
	@docker compose -f ./docker-compose.yaml up -d > /dev/null 2>&1 || true
	@printf "done\n"

stop: ## Tears down the mnemonic infra that was started with 'make start' command
	@printf "Stopping mnemonic.."
	@docker compose down -v --remove-orphans ≈
	@docker rmi migrate/migrate:latest -f > /dev/null 2>&1 || true
	@docker system prune -v > /dev/null 2>&1 || true
	@printf "done\n"

