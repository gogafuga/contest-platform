#!/bin/sh
set -eu
cp /template/Solution.csproj /template/NuGet.Config /template/Solution.sln /work/
cp -R /template/obj /work/obj
printf 'class Program { static void Main() {} }\n' > /work/Program.cs
exec dotnet /opt/csharp-ls/CSharpLanguageServer.dll --solution /work/Solution.sln
