## Dockerfile — multi-target build
##
## Pick service via build-arg:
##   docker build --build-arg SERVICE=order -t checkout/order:dev .
##   docker build --build-arg SERVICE=inventory -t checkout/inventory:dev .

# ---------- Build stage ----------
FROM golang:1.22-alpine AS builder

ARG SERVICE
RUN test -n "$SERVICE" || (echo "ERROR: --build-arg SERVICE is required"; exit 1)

WORKDIR /src

# Cache deps
COPY go.mod go.sum* ./
RUN go mod download

# Build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags='-w -s -X main.version=docker' \
    -trimpath \
    -o /out/service ./cmd/${SERVICE}

# ---------- Runtime stage ----------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /out/service /service

USER nonroot:nonroot
EXPOSE 8080

# Distroless has no shell or curl — health checks must be external (Cloud Run / K8s probe).
ENTRYPOINT ["/service"]
