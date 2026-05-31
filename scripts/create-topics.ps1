# Creates the 10 game topics from kafka/topics/topics.json with the partition,
# replication, cleanup and retention settings from the spec (section 9).
# Idempotent: existing topics are left alone.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$topics = Get-Content "$root\kafka\topics\topics.json" -Raw | ConvertFrom-Json
$broker = "kafka1:19092"

foreach ($t in $topics) {
    $configs = @("--config", "cleanup.policy=$($t.cleanup)")
    if ($null -ne $t.retentionMs) {
        $configs += @("--config", "retention.ms=$($t.retentionMs)")
    }
    Write-Host "Creating $($t.name) (p=$($t.partitions) rf=$($t.replication) $($t.cleanup))..."
    docker exec kafka1 kafka-topics --bootstrap-server $broker --create --if-not-exists `
        --topic $t.name --partitions $t.partitions --replication-factor $t.replication @configs
}

Write-Host "`nTopics now present:" -ForegroundColor Green
docker exec kafka1 kafka-topics --bootstrap-server $broker --list | Select-String "game\."
