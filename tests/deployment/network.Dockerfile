FROM golang:1.27.3-bookworm@sha256:d9b6a1beb9474adc2a1ce842573639b44b2a0dd49b6669f51d3821baf9ec37c9 AS helper-build
WORKDIR /src
COPY go.mod go.sum ./
COPY tests/deployment/cmd/network-fixture/main.go ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/network-fixture ./main.go

FROM debian:bookworm-20261027-slim@sha256:99c0b25c46d0214d5a2091092e7908907dcaecd6a962cde0ac700acee222b6f3
RUN apt-get update \
    && apt-get install --yes --no-install-recommends ca-certificates iproute2 \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /run/fixtures \
    && chmod 0755 /run/fixtures
COPY --from=helper-build /out/network-fixture /usr/local/bin/network-fixture
COPY tests/deployment/setup-network.sh /usr/local/bin/setup-network.sh
RUN chmod 0755 /usr/local/bin/setup-network.sh
VOLUME ["/run/fixtures"]
ENTRYPOINT ["/usr/local/bin/setup-network.sh"]
CMD ["serve"]
