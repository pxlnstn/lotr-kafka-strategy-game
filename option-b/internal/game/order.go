package game

const (
	OrderAssignRoute  = "ASSIGN_ROUTE"
	OrderRedirect     = "REDIRECT_UNIT"
	OrderDestroyRing  = "DESTROY_RING"
	OrderMaiaAbility  = "MAIA_ABILITY"
	OrderBlockPath    = "BLOCK_PATH"
	OrderSearchPath   = "SEARCH_PATH"
	OrderAttackRegion = "ATTACK_REGION"
	OrderReinforce    = "REINFORCE_REGION"
	OrderFortify      = "FORTIFY_REGION"
	OrderDeployNazgul = "DEPLOY_NAZGUL"
)

// Order is the union of every order payload. It arrives as JSON
// from Kafka; only the fields relevant to OrderType are populated.
type Order struct {
	OrderType    string   `json:"orderType"`
	PlayerID     string   `json:"playerId"`
	UnitID       string   `json:"unitId"`
	Turn         int      `json:"turn"`
	PathIds      []string `json:"pathIds,omitempty"`
	NewPathIds   []string `json:"newPathIds,omitempty"`
	TargetPathID string   `json:"targetPathId,omitempty"`
	PathID       string   `json:"pathId,omitempty"`
	TargetRegion string   `json:"targetRegion,omitempty"`
}

// Event is what turn processing produces. Audience drives information hiding
// when these are routed to the browsers.
type Event struct {
	Type string `json:"type"`
	Turn int    `json:"turn"`

	UnitID            string `json:"unitId,omitempty"`
	From              string `json:"from,omitempty"`
	To                string `json:"to,omitempty"`
	PathID            string `json:"pathId,omitempty"`
	Status            string `json:"status,omitempty"`
	SurveillanceLevel int    `json:"surveillanceLevel,omitempty"`
	TempOpenTurns     int    `json:"tempOpenTurns,omitempty"`
	RegionID          string `json:"regionId,omitempty"`
	NewController     string `json:"newController,omitempty"`
	AttackerWon       bool   `json:"attackerWon,omitempty"`
	TrueRegion        string `json:"trueRegion,omitempty"`
	Winner            string `json:"winner,omitempty"`
	Cause             string `json:"cause,omitempty"`

	Audience string `json:"-"` // LIGHT, DARK, or ALL
}

const (
	AudAll   = "ALL"
	AudLight = "LIGHT"
	AudDark  = "DARK"
)

// Event type names (match the Avro schema subjects in section 10).
const (
	EvUnitMoved          = "UnitMoved"
	EvPathStatusChanged  = "PathStatusChanged"
	EvPathCorrupted      = "PathCorrupted"
	EvRegionControl      = "RegionControlChanged"
	EvBattleResolved     = "BattleResolved"
	EvRingBearerMoved    = "RingBearerMoved"
	EvRingBearerDetected = "RingBearerDetected"
	EvRingBearerSpotted  = "RingBearerSpotted"
	EvRouteCompromised   = "RouteCompromised"
	EvGameOver           = "GameOver"
)
