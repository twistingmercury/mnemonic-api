#------------------------------------------------------------
# Stage 1: Run linters, formatters, security scanners, 
# unit tests, then build binary
#------------------------------------------------------------
FROM ghcr.io/twistingmercury/golang-tooling:alpine AS build

ENV CGO_ENABLED=0 \
    GOARCH=amd64 \
    GOOS=linux

ARG BUILD_VER=dev
ARG BUILD_DATE=unknown
ARG BUILD_COMMIT=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download
COPY . .

# Generate Swagger 2.0 docs (required before linters/build — routes.go imports the generated package)
RUN go install github.com/swaggo/swag/cmd/swag@latest
RUN swag init -g cmd/main/main.go -o docs/swagger --parseInternal

# Run linters, formatters, security scanners, etc
RUN goimports -w .
RUN golangci-lint run
RUN govulncheck ./cmd/... ./internal/...
RUN gosec -quiet -exclude-dir=tests ./...
RUN go test -v ./cmd/main/... ./internal/... -coverprofile=coverage.out

ARG LD_FLAGS="-s -w \
    -X 'github.com/twistingmercury/mnemonic-api/internal/version.version=${BUILD_VER}' \
    -X 'github.com/twistingmercury/mnemonic-api/internal/version.buildDate=${BUILD_DATE}' \
    -X 'github.com/twistingmercury/mnemonic-api/internal/version.commit=${BUILD_COMMIT}'"

RUN go build --ldflags "${LD_FLAGS}" -o /out/mnemonic-api cmd/main/main.go


#------------------------------------------------------------
# Stage 2: Runtime - minimal production image
#------------------------------------------------------------
FROM scratch AS final

ARG BUILD_VER=dev
ARG BUILD_DATE=unknown
ARG BUILD_COMMIT=unknown

LABEL org.opencontainers.image.title="mnemonic" \
    org.opencontainers.image.description="Backend server providing routing and pattern retrieval via REST API for ACE CLI" \
    org.opencontainers.image.source="https://github.com/twistingmercury/mnemonic-api" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.authors="Jeremy K. Johnson" \
    org.opencontainers.image.created="${BUILD_DATE}" \
    org.opencontainers.image.revision="${BUILD_COMMIT}" \
    org.opencontainers.image.version="${BUILD_VER}"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /out/mnemonic-api /mnemonic-api

EXPOSE 8080

ENTRYPOINT ["/mnemonic-api"]