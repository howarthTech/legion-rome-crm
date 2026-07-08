# Legion Post CRM — shared image. One image serves every client; a tenant is
# defined entirely by its env file + SQLite volume + published port.
#
# Build:  docker build -t legion-rome-crm .
# Run:    docker run --env-file client.env -p 127.0.0.1:8082:8081 \
#                -v crm-post-x:/data legion-rome-crm

# ---- Build stage ------------------------------------------------------------
# modernc.org/sqlite is pure Go, so CGO is off and the binary is static.
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath -ldflags="-s -w" \
    -o /out/server ./cmd/server

# ---- Runtime stage ----------------------------------------------------------
# alpine (not scratch/distroless) because:
#  - the compose healthcheck shells out to wget (busybox provides it)
#  - tzdata is needed for the quiet-hours guard's time.LoadLocation
#  - ca-certificates for outbound HTTPS to Twilio + the events feed
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget \
    && adduser -D -u 10001 app \
    && mkdir -p /data && chown app:app /data

COPY --from=build /out/server /usr/local/bin/server

USER app

# Container-internal defaults. The CRM binds 0.0.0.0 INSIDE the container; the
# host restricts exposure by publishing to 127.0.0.1:<client-port>:8081. The
# internal port is always 8081 — only the host-side published port varies per
# client. DB lives on the mounted volume.
ENV LISTEN_ADDR=0.0.0.0:8081 \
    DB_PATH=/data/crm.db \
    MEDIA_DIR=/data/media

EXPOSE 8081

ENTRYPOINT ["/usr/local/bin/server"]
