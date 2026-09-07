# syntax=docker/dockerfile:1

ARG SERVICE=infinite
ARG RUNTIME=distroless

FROM golang:1.27-alpine AS build
ARG SERVICE
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" \
      -o /app ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot AS runtime-distroless
COPY --from=build /app /app
USER nonroot:nonroot
ENTRYPOINT ["/app"]

FROM alpine:3.24 AS runtime-alpine
RUN apk add --no-cache ca-certificates
COPY --from=build /app /app
ENTRYPOINT ["/app"]

ARG RUNTIME
FROM runtime-${RUNTIME}
