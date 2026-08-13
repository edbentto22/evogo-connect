# Multi-stage build: compila em Go e roda em Alpine como usuário não-root.
# O shell mínimo é necessário para o Terminal do Coolify executar /app/connect.
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/evogo-connect ./cmd/evogo-connect && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/connect        ./cmd/connect-cli

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && \
    addgroup -S connector && \
    adduser -S -G connector connector
WORKDIR /app
COPY --from=build /out/evogo-connect /app/evogo-connect
COPY --from=build /out/connect        /app/connect
COPY --from=build /src/migrations /app/migrations
USER connector:connector
EXPOSE 9090
ENTRYPOINT ["/app/evogo-connect"]
