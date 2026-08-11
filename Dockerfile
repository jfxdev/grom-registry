FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
COPY backend/api/openapi.yaml /src/backend/api/openapi.yaml
RUN npm run build

FROM golang:1.26.5-alpine AS backend
ARG GROM_VERSION=dev
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN rm -rf internal/webassets/dist && mkdir -p internal/webassets/dist
COPY --from=frontend /src/frontend/dist/ internal/webassets/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${GROM_VERSION}" -o /out/grom ./cmd/grom && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${GROM_VERSION}" -o /out/grom-backup ./cmd/grom-backup

FROM postgres:18-alpine AS postgres-client

FROM alpine:3.22
RUN addgroup -S -g 101 grom && adduser -S -u 100 -G grom grom
WORKDIR /app
COPY --from=backend /out/grom /usr/local/bin/grom
COPY --from=backend /out/grom-backup /usr/local/bin/grom-backup
COPY --from=postgres-client /usr/local/bin/pg_dump /usr/local/bin/pg_restore /usr/local/bin/
COPY --from=postgres-client /usr/local/lib/libpq.so.5 /usr/local/lib/
COPY --chmod=755 deploy/docker/grom-entrypoint.sh /usr/local/bin/grom-entrypoint
COPY deploy/distribution/config.yml /opt/grom/distribution/config.yml
RUN mkdir -p /data /certs /database-backups && chown -R grom:grom /data /certs /database-backups
USER grom
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/grom-entrypoint"]
