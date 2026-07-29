FROM node:24-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
COPY backend/api/openapi.yaml /src/backend/api/openapi.yaml
RUN npm run build

FROM golang:1.26.5-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN rm -rf internal/webassets/dist && mkdir -p internal/webassets/dist
COPY --from=frontend /src/frontend/dist/ internal/webassets/dist/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/grom ./cmd/grom

FROM alpine:3.22
RUN addgroup -S grom && adduser -S -G grom grom
WORKDIR /app
COPY --from=backend /out/grom /usr/local/bin/grom
RUN mkdir -p /data /certs && chown -R grom:grom /data /certs
USER grom
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/grom"]
