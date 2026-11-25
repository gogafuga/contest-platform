FROM golang:1.27-alpine AS agent
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/internal/runner/ ./internal/runner/
COPY backend/cmd/run-agent/ ./cmd/run-agent/
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /run-agent ./cmd/run-agent

FROM python:3.14-slim
COPY --from=agent /run-agent /usr/local/bin/run-agent
ENV HOME=/tmp PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
COPY --chmod=0444 runners/python/run.py /usr/local/lib/contest-python.py
RUN python3 -m py_compile /usr/local/lib/contest-python.py
WORKDIR /work
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/run-agent"]
CMD []
