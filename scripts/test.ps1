# Runs all Go unit tests. No Docker or Kafka required (per spec section 35).
# Pass -race to enable the data-race detector (used for the router tests).
param([switch]$race)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location "$root\option-b"
try {
    if ($race) {
        go test -race ./...
    } else {
        go test ./...
    }
} finally {
    Pop-Location
}
