# syntax=docker/dockerfile:1.6

# ---- build stage ----
FROM golang:1.22-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath

# Cache go.mod first.
COPY go.mod go.sum* ./
RUN go mod download

# Copy the rest and build a static binary.
COPY . .
RUN go build -ldflags "-s -w" -o /out/server ./cmd/server

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
# .env is generated in CI from .env.example + GitHub secrets and baked into
# the image. .env.example is copied as a fallback for local builds.
COPY --from=build /src/.env /app/.env
COPY --from=build /src/.env.example /app/.env.example
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/server"]
