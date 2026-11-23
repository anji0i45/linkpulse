# Start LinkPulse locally. Requires Go 1.22+.
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
Write-Host "Starting LinkPulse at http://localhost:8080 ..."
go run .
