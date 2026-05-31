# Ring of the Middle Earth

A two-player, turn-based strategy game on a Kafka event backbone. The Light
player moves the Ring Bearer secretly toward Mount Doom; the Dark player hunts
it. Two browsers, no AI player.

The full game rules are in [docs/RULES.md](docs/RULES.md), and the system design
is in [docs/architecture.pdf](docs/architecture.pdf).

## Technology choice

Option B — Go. The game engine, the order-validation and route-risk services,
and the HTTP/SSE layer are all Go; the UI is vanilla JavaScript. A pure-Go
Kafka client (franz-go) is used, so the build needs no C toolchain. Kafka (3
brokers, KRaft mode), the Confluent Schema Registry and a Kafka web UI run in
Docker, alongside three interchangeable engine instances behind an nginx load
balancer.

See `docs/architecture.pdf` for the full design, the paradigm justification and
the reflection.

## Layout

```
docker-compose.yml      3x Kafka + Schema Registry + Kafka UI + 3 engines + nginx
config/                 map.conf (22 regions, 37 paths) and units.conf (13 units)
kafka/schemas/          Avro .avsc files          kafka/topics/  topic definitions
option-b/               the Go module (module path: rotme)
  cmd/engine            entrypoint            internal/game     rules + 13-step turn
  internal/config       map + graph           internal/engine   pipeline + leader election
  internal/kafka        client + avro serde   internal/analysis route risk + intercept
ui/                     index.html, game.js, style.css
scripts/                up / down / test / topics / schemas (PowerShell)
deploy/nginx.conf       load balancer
```

## Prerequisites

Docker Desktop, Go 1.22+, Git. (Windows: the scripts are PowerShell.)

## Run

```powershell
powershell -File scripts/up.ps1          # or: make up    — builds and starts everything
#   Light: http://localhost:8080/?side=light
#   Dark:  http://localhost:8080/?side=dark
#   Kafka UI: http://localhost:8088

powershell -File scripts/test.ps1        # or: make test       — unit tests, no Docker
powershell -File scripts/test-race.ps1   # or: make test-race  — race tests in a Linux container
powershell -File scripts/down.ps1        # or: make down
```

Click Start Game, pick a unit, choose an order and target, Submit order, then
click Advance Turn to play out each turn. For the Ring Bearer, choose Assign
Route with Mount Doom as the destination and keep advancing - it walks there
one region per turn; once it arrives, the Destroy Ring order appears.

## Fault tolerance

The three engines form one consumer group; one is elected leader and runs turn
processing. `docker stop go-2` triggers a rebalance — if it was the leader,
another instance is promoted and recovers the world from Kafka, and the game
continues. `docker start go-2` rejoins it.
