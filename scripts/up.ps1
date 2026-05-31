# Brings up the whole system: 3 Kafka brokers, Schema Registry, Kafka UI, the
# three Go engine instances, and the nginx load balancer. Then creates the
# topics and registers the Avro schemas. Idempotent.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$compose = "$root\docker-compose.yml"

Write-Host "Building and starting the stack..." -ForegroundColor Cyan
docker compose -f $compose up -d --build

function Wait-Healthy($name, $timeoutSec) {
    $deadline = (Get-Date).AddSeconds($timeoutSec)
    Write-Host "Waiting for $name..." -NoNewline
    while ($true) {
        $state = docker inspect --format "{{.State.Health.Status}}" $name 2>$null
        if ($state -eq "healthy") { Write-Host " OK" -ForegroundColor Green; return $true }
        if ((Get-Date) -gt $deadline) { Write-Host " TIMEOUT" -ForegroundColor Red; return $false }
        Start-Sleep -Seconds 3; Write-Host "." -NoNewline
    }
}

foreach ($svc in @("kafka1", "kafka2", "kafka3", "schema-registry")) {
    if (-not (Wait-Healthy $svc 180)) { docker logs --tail 30 $svc; exit 1 }
}

Write-Host "`nCreating topics..." -ForegroundColor Cyan
& "$root\scripts\create-topics.ps1" | Out-Null
Write-Host "Registering schemas..." -ForegroundColor Cyan
& "$root\scripts\register-schemas.ps1" | Out-Null

foreach ($svc in @("go-1", "go-2", "go-3")) { Wait-Healthy $svc 120 | Out-Null }

Write-Host "`nSystem is up." -ForegroundColor Green
Write-Host "  Game (Light): http://localhost:8080/?side=light"
Write-Host "  Game (Dark):  http://localhost:8080/?side=dark"
Write-Host "  Kafka UI:     http://localhost:8088"
Write-Host "  Schema Reg:   http://localhost:8081"
