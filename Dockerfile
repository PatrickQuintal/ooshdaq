# Keep the human-readable version and the digest together. Dependabot updates
# the tag; reviewers verify and update the multi-platform digest.
FROM docker.io/library/golang:1.27.0-alpine3.23@sha256:3747dcba41c8b0db3211fda4db61638b980e17ac5bb3c94460a975a9cfe19395 AS build

WORKDIR /src

ENV GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
      -mod=readonly \
      -trimpath \
      -buildvcs=false \
      -ldflags="-s -w -buildid=" \
      -o /out/ooshdaq \
      ./cmd/ooshdaq \
    && mkdir -p /empty-data /empty-backups

FROM scratch

COPY --from=build --chown=65532:65532 /out/ooshdaq /ooshdaq
COPY --from=build --chown=65532:65532 /empty-data /data
COPY --from=build --chown=65532:65532 /empty-backups /backups
COPY --chown=65532:65532 LICENSE NOTICE /licenses/
COPY --chown=65532:65532 LICENSES /licenses/third-party

LABEL org.opencontainers.image.title="\$OOSHDAQ" \
      org.opencontainers.image.description="Private-community collectible exchange matcher" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="https://github.com/PatrickQuintal/ooshdaq"

USER 65532:65532
WORKDIR /

ENV OOSHDAQ_ADDR=:8080 \
    OOSHDAQ_DATA_PATH=/data/ooshdaq.sqlite \
    OOSHDAQ_BACKUP_DIR=/backups \
    OOSHDAQ_BACKUP_INTERVAL=24h \
    OOSHDAQ_BACKUP_RETENTION_COUNT=14

EXPOSE 8080

ENTRYPOINT ["/ooshdaq"]
