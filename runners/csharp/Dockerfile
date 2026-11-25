FROM golang:1.27-alpine AS agent
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
COPY backend/internal/runner/ ./internal/runner/
COPY backend/cmd/run-agent/ ./cmd/run-agent/
RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -o /run-agent ./cmd/run-agent

FROM mcr.microsoft.com/dotnet/sdk:10.0
COPY --from=agent /run-agent /usr/local/bin/run-agent
ENV HOME=/tmp DOTNET_CLI_HOME=/tmp/dotnet NUGET_PACKAGES=/tmp/nuget \
    DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 DOTNET_EnableDiagnostics=0 \
    DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE=true DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1 \
    MSBuildEnableWorkloadResolver=false DOTNET_PROCESSOR_COUNT=1 \
    DOTNET_GCHeapHardLimit=0x10000000 DOTNET_GCConserveMemory=9 DOTNET_gcServer=0
COPY --chmod=0555 runners/csharp/prepare-compiler.sh /usr/local/bin/prepare-compiler
COPY --chmod=0555 runners/csharp/compile.sh /usr/local/bin/compile-csharp
# Validate the fixed compiler configuration with a trusted fixture at image build.
RUN /usr/local/bin/prepare-compiler \
    && mkdir -p /work /exec \
    && printf '%s\n' 'class Program { static void Main() { System.Console.Write("compiler-ready"); } }' > /work/Program.cs \
    && /usr/local/bin/compile-csharp \
    && test "$(/usr/bin/dotnet exec --runtimeconfig /opt/Solution.runtimeconfig.json /exec/Solution.dll)" = compiler-ready \
    && rm /work/Program.cs /exec/Solution.dll /usr/local/bin/prepare-compiler
WORKDIR /work
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/run-agent"]
CMD []
