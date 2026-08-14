# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.24
ARG NODE_VERSION=22

FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /src/web
COPY web/package.json ./package.json
COPY web/index.html ./index.html
COPY web/scripts ./scripts
COPY web/src ./src
RUN npm test && npm run build

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates git
COPY go.mod ./go.mod
COPY . .
COPY --from=web /src/web/dist ./web/dist
COPY --from=web /src/web/dist ./internal/server/web/dist
ARG RESTRICTED=true
ARG VERSION=0.1.0-alpha
RUN if [ "$RESTRICTED" = "true" ]; then \
      CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault ./cmd/dbvault && \
      CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvaultd ./cmd/dbvaultd && \
      CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-agent ./cmd/dbvault-agent && \
      CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-controller ./cmd/dbvault-controller && \
      CGO_ENABLED=0 go build -tags=restricted -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-operator ./cmd/dbvault-operator; \
    else \
      go mod download && go mod verify && \
      CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault ./cmd/dbvault && \
      CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvaultd ./cmd/dbvaultd && \
      CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-agent ./cmd/dbvault-agent && \
      CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-controller ./cmd/dbvault-controller && \
      CGO_ENABLED=1 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/dbvault-operator ./cmd/dbvault-operator; \
    fi

FROM gcr.io/distroless/base-debian12:nonroot AS runtime
LABEL org.opencontainers.image.title="DBVault"
LABEL org.opencontainers.image.description="Self-contained database backup and verified recovery appliance"
LABEL org.opencontainers.image.licenses="Apache-2.0"
LABEL org.opencontainers.image.source="https://github.com/dbvault/dbvault"
COPY --from=build /out/ /usr/local/bin/
USER nonroot:nonroot
VOLUME ["/var/lib/dbvault"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/usr/local/bin/dbvault", "doctor"]
ENTRYPOINT ["/usr/local/bin/dbvault"]
CMD ["server", "--listen", "0.0.0.0:8080", "--data-dir", "/var/lib/dbvault"]
