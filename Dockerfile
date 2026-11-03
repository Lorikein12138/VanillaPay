FROM node:24.22.2-bookworm-slim@sha256:c86ecead9e55a5210ac1727de38a4ca840bf03dcaf34dc9749addbf383b1920c AS web-build
WORKDIR /workspace
COPY web/package.json web/package-lock.json ./web/
RUN npm --prefix ./web ci
COPY web ./web
WORKDIR /workspace/web
RUN npm run build

FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web-build /workspace/internal/webassets/dist ./internal/webassets/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server

FROM debian:bookworm-20261027-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3 AS runtime
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /var/lib/vanillapay \
    && chown 65532:65532 /var/lib/vanillapay
COPY --from=go-build --chown=65532:65532 /out/server /usr/local/bin/server
ENV LISTEN_ADDR=:8080 \
    DATA_DIR=/var/lib/vanillapay
VOLUME ["/var/lib/vanillapay"]
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/server"]
CMD ["serve"]
