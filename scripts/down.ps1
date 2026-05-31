# Stops and removes all containers. Pass -v to also wipe Kafka data volumes.
param([switch]$v)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

if ($v) {
    Write-Host "Stopping stack and removing data volumes..." -ForegroundColor Yellow
    docker compose -f "$root\docker-compose.yml" down -v
} else {
    Write-Host "Stopping stack (data volumes preserved)..." -ForegroundColor Cyan
    docker compose -f "$root\docker-compose.yml" down
}
