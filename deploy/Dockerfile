# Multi-stage build: compila em golang:1.24, roda em distroless.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/evogo-connect ./cmd/evogo-connect && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/connect        ./cmd/connect-cli

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/evogo-connect /app/evogo-connect
COPY --from=build /out/connect        /app/connect
COPY migrations /app/migrations
USER nonroot:nonroot
EXPOSE 9090
ENTRYPOINT ["/app/evogo-connect"]
