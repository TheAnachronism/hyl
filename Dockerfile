FROM node:22-alpine AS web
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY frontend/ ./
RUN npm run typecheck && npm run build

# The build and runtime stages must stay on the same Alpine series: libvips is
# linked by CGO here and loaded again in the runtime image.
FROM golang:1.27-alpine3.24 AS build
ARG VERSION=dev
RUN apk add --no-cache build-base pkgconf vips-dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/frontend/dist ./frontend/dist
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/hyl .

FROM alpine:3.24
RUN apk add --no-cache vips vips-heif ca-certificates tzdata \
    && adduser -D -u 10001 hyl && mkdir -p /data && chown -R hyl /data
COPY --from=build /out/hyl /usr/local/bin/hyl
USER hyl
ENV HYL_DATA_DIR=/data \
    HYL_LISTEN=:8080
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/hyl"]
