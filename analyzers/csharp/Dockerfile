FROM mcr.microsoft.com/dotnet/sdk:10.0 AS server-build
ARG CSHARP_READYTORUN=true
# Pin both the upstream release and its archive contents.
ADD --checksum=sha256:a9a34dccff8b4aee2c1f0b8c3f2a3fd0361dfd55398385f465cc4e8f238045b4 \
    https://codeload.github.com/razzmatazz/csharp-language-server/tar.gz/refs/tags/0.27.0 \
    /tmp/csharp-ls-source.tar.gz
COPY build-server.sh /tmp/build-server.sh
RUN CSHARP_READYTORUN="$CSHARP_READYTORUN" sh /tmp/build-server.sh /out /tmp/csharp-ls-source.tar.gz

FROM mcr.microsoft.com/dotnet/sdk:10.0
COPY --from=server-build /out /opt/csharp-ls
ENV HOME=/tmp DOTNET_CLI_HOME=/tmp/dotnet NUGET_PACKAGES=/tmp/nuget \
    DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1 DOTNET_EnableDiagnostics=0 \
    DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE=true DOTNET_SKIP_FIRST_TIME_EXPERIENCE=1 \
    MSBuildEnableWorkloadResolver=false DOTNET_PROCESSOR_COUNT=1 \
    DOTNET_GCHeapHardLimit=0x10000000 DOTNET_GCConserveMemory=9 DOTNET_gcServer=0
COPY Solution.csproj NuGet.Config /template/
# Prepare the fixed solution/restore assets once, not on every editor connection.
RUN mkdir -p /work \
    && cp /template/Solution.csproj /template/NuGet.Config /work/ \
    && printf 'class Program { static void Main() {} }\n' > /work/Program.cs \
    && cd /work \
    && dotnet new sln --name Solution --format sln --force \
    && dotnet sln /work/Solution.sln add /work/Solution.csproj \
    && dotnet restore /work/Solution.csproj --configfile /work/NuGet.Config \
    && dotnet build /work/Solution.csproj --no-restore -c Debug -p:UseSharedCompilation=false \
    && cp /work/Solution.sln /template/ \
    && cp -R /work/obj /template/obj \
    && rm -rf /work/*
RUN dotnet /opt/csharp-ls/CSharpLanguageServer.dll --version
COPY --chmod=0555 start.sh /usr/local/bin/start-analyzer
WORKDIR /work
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/start-analyzer"]

CMD []
