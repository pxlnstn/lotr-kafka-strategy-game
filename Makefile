# Thin Makefile that delegates to the PowerShell scripts so the spec's
# `make up` / `make test` work if GNU Make is installed. On Windows without
# Make, run the scripts directly, e.g.  powershell -File scripts/up.ps1
#
# These targets shell out to PowerShell so behaviour is identical either way.

.PHONY: up down down-clean test test-race topics schemas logs ps

up:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/up.ps1

down:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/down.ps1

down-clean:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/down.ps1 -v

test:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test.ps1

test-race:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-race.ps1

topics:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/create-topics.ps1

logs:
	docker compose logs -f --tail=50

ps:
	docker compose ps
