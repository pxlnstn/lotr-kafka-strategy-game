# Runs the race-detector tests. The Go race detector needs cgo (a C compiler),
# which native Windows Go lacks, so we run it inside a Linux Go container.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
docker run --rm -v "${root}:/repo" -w /repo/option-b golang:1.26 go test -race ./...
