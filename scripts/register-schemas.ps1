# Registers every Avro schema in kafka/schemas with the Schema Registry,
# using the subject mapping in subjects.json. Topics that carry a single
# record type use the {topic}-value subject; topics that carry several event
# types use a record-name subject.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$sr = "http://localhost:8081"
$schemaDir = "$root\kafka\schemas"
$map = Get-Content "$schemaDir\subjects.json" -Raw | ConvertFrom-Json

foreach ($m in $map) {
    $schema = [IO.File]::ReadAllText("$schemaDir\$($m.file)")
    $body = @{ schema = $schema; schemaType = "AVRO" } | ConvertTo-Json
    $resp = Invoke-RestMethod -Uri "$sr/subjects/$($m.subject)/versions" -Method Post `
        -ContentType "application/vnd.schemaregistry.v1+json" -Body $body
    Write-Host ("{0,-34} -> id {1}" -f $m.subject, $resp.id)
}

Write-Host "`nRegistered subjects:" -ForegroundColor Green
Invoke-RestMethod -Uri "$sr/subjects" | Sort-Object | ForEach-Object { "  $_" }
