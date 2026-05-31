package engine

import "rotme/internal/game"

// Avro DTOs. Field tags match the schema field names in kafka/schemas.

type orderSubmittedMsg struct {
	PlayerID  string `avro:"playerId"`
	UnitID    string `avro:"unitId"`
	OrderType string `avro:"orderType"`
	Payload   []byte `avro:"payload"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type orderValidatedMsg struct {
	PlayerID       string `avro:"playerId"`
	UnitID         string `avro:"unitId"`
	OrderType      string `avro:"orderType"`
	Payload        []byte `avro:"payload"`
	Turn           int    `avro:"turn"`
	Timestamp      int64  `avro:"timestamp"`
	RouteRiskScore *int   `avro:"routeRiskScore"` // V2 field; null for non-route orders
}

type unitMovedMsg struct {
	UnitID    string `avro:"unitId"`
	From      string `avro:"from"`
	To        string `avro:"to"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type pathStatusChangedMsg struct {
	PathID            string `avro:"pathId"`
	NewStatus         string `avro:"newStatus"`
	SurveillanceLevel int    `avro:"surveillanceLevel"`
	TempOpenTurns     int    `avro:"tempOpenTurns"`
	Turn              int    `avro:"turn"`
	Timestamp         int64  `avro:"timestamp"`
}

type pathCorruptedMsg struct {
	PathID    string `avro:"pathId"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type regionControlMsg struct {
	RegionID      string `avro:"regionId"`
	NewController string `avro:"newController"`
	Turn          int    `avro:"turn"`
	Timestamp     int64  `avro:"timestamp"`
}

type battleResolvedMsg struct {
	RegionID    string `avro:"regionId"`
	AttackerWon bool   `avro:"attackerWon"`
	Turn        int    `avro:"turn"`
	Timestamp   int64  `avro:"timestamp"`
}

type ringMovedMsg struct {
	TrueRegion string `avro:"trueRegion"`
	Turn       int    `avro:"turn"`
	Timestamp  int64  `avro:"timestamp"`
}

type ringDetectedMsg struct {
	RegionID  string `avro:"regionId"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type ringSpottedMsg struct {
	PathID    string `avro:"pathId"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type gameOverMsg struct {
	Winner    string `avro:"winner"`
	Cause     string `avro:"cause"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

type regionSnap struct {
	RegionID     string `avro:"regionId" json:"regionId"`
	ControlledBy string `avro:"controlledBy" json:"controlledBy"`
	ThreatLevel  int    `avro:"threatLevel" json:"threatLevel"`
	Fortified    bool   `avro:"fortified" json:"fortified"`
}

type unitSnap struct {
	UnitID   string `avro:"unitId" json:"unitId"`
	Region   string `avro:"region" json:"region"`
	Strength int    `avro:"strength" json:"strength"`
	Status   string `avro:"status" json:"status"`
}

// pathSnap is JSON-only, used by GET /game/state (paths are not in the broadcast schema).
type pathSnap struct {
	PathID            string `json:"pathId"`
	Status            string `json:"status"`
	SurveillanceLevel int    `json:"surveillanceLevel"`
	TempOpenTurns     int    `json:"tempOpenTurns"`
}

// APIState is the JSON view returned by GET /game/state and built from the
// hub's Kafka-rebuilt cache. For the Dark Side, RingBearerRegion is blank.
type APIState struct {
	Turn             int          `json:"turn"`
	RingBearerRegion string       `json:"ringBearerRegion"`
	Units            []unitSnap   `json:"units"`
	Regions          []regionSnap `json:"regions"`
	Paths            []pathSnap   `json:"paths"`
	Winner           string       `json:"winner"`
	GameOver         bool         `json:"gameOver"`
	Started          bool         `json:"started"`
}

type worldSnapshotMsg struct {
	Turn      int          `avro:"turn" json:"turn"`
	Regions   []regionSnap `avro:"regions" json:"regions"`
	Units     []unitSnap   `avro:"units" json:"units"`
	Timestamp int64        `avro:"timestamp" json:"timestamp"`
}

type gameSessionMsg struct {
	GameID    string  `avro:"gameId"`
	Turn      int     `avro:"turn"`
	Phase     string  `avro:"phase"`
	Winner    *string `avro:"winner"`
	WorldJSON *string `avro:"worldJson"` // full state for leader recovery; not browser-facing
	Timestamp int64   `avro:"timestamp"`
}

type dlqMsg struct {
	OriginalTopic string `avro:"originalTopic"`
	Partition     int    `avro:"partition"`
	Offset        int64  `avro:"offset"`
	ErrorCode     string `avro:"errorCode"`
	ErrorMessage  string `avro:"errorMessage"`
	RawPayload    []byte `avro:"rawPayload"`
	Timestamp     int64  `avro:"timestamp"`
}

// Topic names.
const (
	topicOrdersRaw       = "game.orders.raw"
	topicOrdersValidated = "game.orders.validated"
	topicEventsUnit      = "game.events.unit"
	topicEventsRegion    = "game.events.region"
	topicEventsPath      = "game.events.path"
	topicSession         = "game.session"
	topicBroadcast       = "game.broadcast"
	topicRingPosition    = "game.ring.position"
	topicRingDetection   = "game.ring.detection"
	topicDLQ             = "game.dlq"
)

// Logical schema names -> subject. Resolved to ids at startup.
var schemaSubjects = map[string]string{
	"OrderSubmitted":       "game.orders.raw-value",
	"OrderValidated":       "game.orders.validated-value",
	"UnitMoved":            "UnitMoved-value",
	"PathStatusChanged":    "PathStatusChanged-value",
	"PathCorrupted":        "PathCorrupted-value",
	"RegionControlChanged": "RegionControlChanged-value",
	"BattleResolved":       "BattleResolved-value",
	"RingBearerMoved":      "game.ring.position-value",
	"RingBearerDetected":   "RingBearerDetected-value",
	"RingBearerSpotted":    "RingBearerSpotted-value",
	"WorldStateSnapshot":   "game.broadcast-value",
	"GameOver":             "game.broadcast-GameOver",
	"GameSession":          "game.session-value",
	"DLQEntry":             "game.dlq-value",
}

// routeTopic maps an event type to its destination topic and schema name. The
// bool is false for events that are not published to Kafka (e.g. SSE-only).
func routeTopic(t string) (topic, schema string, ok bool) {
	switch t {
	case game.EvUnitMoved:
		return topicEventsUnit, "UnitMoved", true
	case game.EvPathStatusChanged:
		return topicEventsPath, "PathStatusChanged", true
	case game.EvPathCorrupted:
		return topicEventsPath, "PathCorrupted", true
	case game.EvRegionControl:
		return topicEventsRegion, "RegionControlChanged", true
	case game.EvBattleResolved:
		return topicEventsRegion, "BattleResolved", true
	case game.EvRingBearerMoved:
		return topicRingPosition, "RingBearerMoved", true
	case game.EvRingBearerDetected:
		return topicRingDetection, "RingBearerDetected", true
	case game.EvRingBearerSpotted:
		return topicRingDetection, "RingBearerSpotted", true
	default:
		return "", "", false
	}
}
