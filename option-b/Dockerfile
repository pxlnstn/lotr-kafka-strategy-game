# Build context is the repo root (so we can copy config/ and ui/ too).
FROM golang:1.26 AS build
WORKDIR /src
COPY option-b/go.mod option-b/go.sum ./
RUN go mod download
COPY option-b/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /engine ./cmd/engine

FROM alpine:3.20
WORKDIR /app
COPY --from=build /engine /app/engine
COPY config/ /app/config/
COPY ui/ /app/ui/
ENV CONFIG_DIR=/app/config
ENV UI_DIR=/app/ui
EXPOSE 8080
ENTRYPOINT ["/app/engine"]
