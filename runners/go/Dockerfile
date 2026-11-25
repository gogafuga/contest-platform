FROM golang:1.27-alpine AS agent
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/internal/runner/ ./internal/runner/
COPY backend/cmd/run-agent/ ./cmd/run-agent/
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /run-agent ./cmd/run-agent

FROM golang:1.27-alpine
COPY runners/go/warm.go /tmp/warm.go
RUN CGO_ENABLED=0 GOTOOLCHAIN=local GOCACHE=/opt/go-cache \
    go build -trimpath -o /tmp/warm /tmp/warm.go \
    && rm /tmp/warm /tmp/warm.go && chmod -R a+rX /opt/go-cache
COPY --chmod=0555 runners/go/start.sh /usr/local/bin/start-runner
COPY --from=agent /run-agent /usr/local/bin/run-agent
ENV HOME=/tmp GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOTELEMETRY=off \
    GOWORK=off GOENV=off GOPACKAGESDRIVER=off CGO_ENABLED=0 \
    GOPATH=/tmp/go GOCACHE=/tmp/go-cache GOMODCACHE=/tmp/go-modules GOMAXPROCS=2
WORKDIR /work
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/start-runner"]
CMD []
