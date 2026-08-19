#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJ_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
APP_ROOT="${PROJ_ROOT}/src"
LOCAL=${LOCAL:-0}

BUILD_VER="${BUILD_VER:-$(git -C "${PROJ_ROOT}" describe --tags --always --dirty 2>/dev/null || echo 'dev')}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
BUILD_COMMIT="${BUILD_COMMIT:-$(git -C "${PROJ_ROOT}" rev-parse HEAD 2>/dev/null || echo 'unknown')}"

IMAGE_NAME="${IMAGE_NAME:-ghcr.io/twistingmercury/mnemonic-api}"
if [[ -z "${IMAGE_TAG:-}" ]]; then
    if [[ "${BUILD_COMMIT}" == "unknown" ]]; then
        IMAGE_TAG="dev"
    else
        IMAGE_TAG="sha-${BUILD_COMMIT}"
    fi
fi

E2E_COMPOSE_FILE="${APP_ROOT}/tests/docker-compose.yaml"


build_api() {
    printf "\n=== starting image build, version %s ===\n" "${BUILD_VER}"

    local image_tags=(--tag "${IMAGE_NAME}:${IMAGE_TAG}")
    if [[ "${IMAGE_TAG}" != "latest" ]]; then
        image_tags+=(--tag "${IMAGE_NAME}:latest")
    fi

    docker build --rm --no-cache \
        --file "${SCRIPT_DIR}/Dockerfile" \
        --build-arg BUILD_VER="${BUILD_VER}" \
        --build-arg BUILD_DATE="${BUILD_DATE}" \
        --build-arg BUILD_COMMIT="${BUILD_COMMIT}" \
        --target final \
        "${image_tags[@]}" \
        "${PROJ_ROOT}"

    printf "\nImage: %s:%s\n" "${IMAGE_NAME}" "${IMAGE_TAG}"
    docker images "${IMAGE_NAME}:${IMAGE_TAG}" --format "Size: {{.Size}}"

    return 0
}

e2e_tests() {
    printf "\n=== starting end-to-end tests ===\n"

    cleanup() {
        docker compose -f "${E2E_COMPOSE_FILE}" down -v --remove-orphans > /dev/null 2>&1 || true

        if [[ "${LOCAL}" == "1" ]]; then
            docker rmi tests_mnemonic_tests:latest -f > /dev/null 2>&1 || true
            docker system prune -f > /dev/null 2>&1 || true
        fi
    }
    trap cleanup EXIT

    printf "Starting infrastructure services...\n"
    if ! docker compose -f "${E2E_COMPOSE_FILE}" up -d postgres neo4j rabbitmq; then
        printf "ERROR: Failed to start infrastructure services\n" >&2
        return 1
    fi

    docker compose -f "${E2E_COMPOSE_FILE}" up \
        --build \
        --abort-on-container-exit \
        --exit-code-from mnemonic_tests \
        mnemonic_api mnemonic_tests

    trap - EXIT
    cleanup

    return 0
}

main() {
    build_api

    e2e_tests

    return 0
}

main "$@"
