# Build a static binary, then ship it on a minimal base. The resulting image is a few
# megabytes, which matters on free-tier container hosts with tight memory limits.
# These are the Docker Official Images, pulled from the ECR Public mirror so GitHub
# Actions does not hit Docker Hub's anonymous rate limit.
FROM public.ecr.aws/docker/library/golang:1.26-alpine AS build

WORKDIR /src

# Dependencies are cached separately from source so code edits do not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/nimbus-api ./cmd/api

FROM public.ecr.aws/docker/library/alpine:3.22

# Presigned URL generation and TLS calls to R2 need root certificates and correct time.
RUN apk add --no-cache ca-certificates tzdata curl \
    && adduser -D -u 10001 nimbus

COPY --from=build /out/nimbus-api /usr/local/bin/nimbus-api

USER nimbus
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD curl -fsS http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/nimbus-api"]
