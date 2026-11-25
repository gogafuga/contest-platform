FROM golang:1.27-alpine AS build

RUN CGO_ENABLED=0 GOBIN=/out \
    go install golang.org/x/tools/gopls@v0.24.0

FROM golang:1.27-alpine

COPY --from=build /out/gopls /usr/local/bin/gopls
COPY warm.go /tmp/warm.go
RUN CGO_ENABLED=0 GOCACHE=/opt/go-cache go build -o /tmp/warm /tmp/warm.go \
    && chmod -R u+rwX,go+rX /opt/go-cache \
    && tar -C /opt/go-cache -cf /opt/go-cache.tar . \
    && rm -rf /opt/go-cache /tmp/warm /tmp/warm.go

ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    GOSUMDB=off \
    GOTELEMETRY=off \
    GOWORK=off \
    GOENV=off \
    CGO_ENABLED=0 \
    HOME=/tmp \
    GOPATH=/tmp/go \
    GOCACHE=/tmp/go-cache \
    GOMODCACHE=/tmp/go-modules \
    GOFLAGS=-mod=readonly \
    GOMAXPROCS=2

COPY --chmod=0555 start.sh /usr/local/bin/start-analyzer

WORKDIR /work

USER 65532:65532

ENTRYPOINT ["/usr/local/bin/start-analyzer"]
CMD []
