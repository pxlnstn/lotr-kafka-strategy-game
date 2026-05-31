# Demonstrates schema evolution (spec K3): adds the nullable routeRiskScore
# field to OrderValidated as V2 on the game.orders.validated-value subject.
# Checks backward compatibility first, then registers. V1 consumers keep
# working because the new field has a default of null.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$sr = "http://localhost:8081"
$subject = "game.orders.validated-value"
$schema = [IO.File]::ReadAllText("$root\kafka\schemas\OrderValidated-v2.avsc")
$body = @{ schema = $schema; schemaType = "AVRO" } | ConvertTo-Json

$check = Invoke-RestMethod -Uri "$sr/compatibility/subjects/$subject/versions/latest" -Method Post `
    -ContentType "application/vnd.schemaregistry.v1+json" -Body $body
Write-Host "Backward compatible with latest: $($check.is_compatible)"
if (-not $check.is_compatible) { throw "V2 is not compatible; aborting." }

$resp = Invoke-RestMethod -Uri "$sr/subjects/$subject/versions" -Method Post `
    -ContentType "application/vnd.schemaregistry.v1+json" -Body $body
Write-Host "Registered V2 -> id $($resp.id)"

Write-Host "`nVersions on ${subject}:"
Invoke-RestMethod -Uri "$sr/subjects/$subject/versions"
