package kafka

import "testing"

type orderValidated struct {
	PlayerID  string `avro:"playerId"`
	UnitID    string `avro:"unitId"`
	OrderType string `avro:"orderType"`
	Payload   []byte `avro:"payload"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

const ovSchema = `{
  "type":"record","name":"OrderValidated","namespace":"rotr.events",
  "fields":[
    {"name":"playerId","type":"string"},
    {"name":"unitId","type":"string"},
    {"name":"orderType","type":"string"},
    {"name":"payload","type":"bytes"},
    {"name":"turn","type":"int"},
    {"name":"timestamp","type":"long"}
  ]
}`

func TestAvroRoundTrip(t *testing.T) {
	s := NewAvroSerde()
	if err := s.Register(2, ovSchema); err != nil {
		t.Fatal(err)
	}

	in := orderValidated{
		PlayerID: "light", UnitID: "ring-bearer", OrderType: "ASSIGN_ROUTE",
		Payload: []byte(`{"pathIds":["shire-to-bree"]}`), Turn: 1, Timestamp: 1717000000000,
	}
	wire, err := s.Encode(2, in)
	if err != nil {
		t.Fatal(err)
	}
	if wire[0] != magicByte {
		t.Fatalf("missing magic byte, got %x", wire[0])
	}

	var out orderValidated
	id, err := s.Decode(wire, &out)
	if err != nil {
		t.Fatal(err)
	}
	if id != 2 {
		t.Fatalf("schema id = %d, want 2", id)
	}
	if out.UnitID != in.UnitID || out.OrderType != in.OrderType || out.Turn != in.Turn {
		t.Fatalf("roundtrip mismatch: %+v", out)
	}
}

func TestDecodeGeneric(t *testing.T) {
	s := NewAvroSerde()
	if err := s.Register(2, ovSchema); err != nil {
		t.Fatal(err)
	}
	in := orderValidated{PlayerID: "dark", UnitID: "witch-king", OrderType: "BLOCK_PATH", Turn: 4, Timestamp: 1}
	wire, _ := s.Encode(2, in)
	m, id, err := s.DecodeGeneric(wire)
	if err != nil {
		t.Fatal(err)
	}
	if id != 2 || m["unitId"] != "witch-king" || m["orderType"] != "BLOCK_PATH" {
		t.Fatalf("generic decode wrong: id=%d m=%v", id, m)
	}
}

func TestDecodeRejectsRawJSON(t *testing.T) {
	s := NewAvroSerde()
	var out orderValidated
	if _, err := s.Decode([]byte(`{"not":"avro"}`), &out); err == nil {
		t.Fatal("expected wire-format error on non-avro payload")
	}
}
