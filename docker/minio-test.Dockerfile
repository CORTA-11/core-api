# syntax=docker/dockerfile:1

FROM golang:1.26.6-alpine AS build

# Official RELEASE.2025-10-15T17-29-55Z, pinned to its source commit.
# Build locally because the upstream prebuilt images are no longer pullable.
RUN CGO_ENABLED=0 GOBIN=/out go install github.com/minio/minio@9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a

FROM alpine:3.23
RUN apk add --no-cache ca-certificates curl
COPY --from=build /out/minio /usr/local/bin/minio
ENTRYPOINT ["minio"]
