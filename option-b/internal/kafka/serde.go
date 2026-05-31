// Package kafka holds the messaging glue: an Avro serializer in Confluent
// wire format and thin producer/consumer wrappers over a pure-Go client.
package kafka

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/hamba/avro/v2"
)

// Confluent wire format: 1 magic byte (0x00) + 4-byte big-endian schema id +
// Avro binary payload.
const magicByte = 0x00

var errBadWireFormat = errors.New("kafka: payload is not in confluent avro wire format")

// AvroSerde encodes and decodes records against schemas keyed by their
// registry id. The id-to-schema mapping is populated from the Schema Registry
// at startup (or hardcoded in tests).
type AvroSerde struct {
	schemas map[int]avro.Schema
}

func NewAvroSerde() *AvroSerde {
	return &AvroSerde{schemas: make(map[int]avro.Schema)}
}

// Register parses a schema string and stores it under its registry id.
func (s *AvroSerde) Register(id int, schema string) error {
	sc, err := avro.Parse(schema)
	if err != nil {
		return fmt.Errorf("parse schema id %d: %w", id, err)
	}
	s.schemas[id] = sc
	return nil
}

func (s *AvroSerde) Encode(id int, v any) ([]byte, error) {
	sc, ok := s.schemas[id]
	if !ok {
		return nil, fmt.Errorf("kafka: no schema registered for id %d", id)
	}
	payload, err := avro.Marshal(sc, v)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 5+len(payload))
	out[0] = magicByte
	binary.BigEndian.PutUint32(out[1:5], uint32(id))
	copy(out[5:], payload)
	return out, nil
}

// DecodeGeneric decodes any registered record into a map, for forwarding to
// the browser as JSON without knowing the concrete Go type.
func (s *AvroSerde) DecodeGeneric(data []byte) (map[string]any, int, error) {
	if len(data) < 5 || data[0] != magicByte {
		return nil, 0, errBadWireFormat
	}
	id := int(binary.BigEndian.Uint32(data[1:5]))
	sc, ok := s.schemas[id]
	if !ok {
		return nil, id, fmt.Errorf("kafka: no schema registered for id %d", id)
	}
	var m map[string]any
	if err := avro.Unmarshal(sc, data[5:], &m); err != nil {
		return nil, id, err
	}
	return m, id, nil
}

// Decode reads the schema id from the header and unmarshals into v.
func (s *AvroSerde) Decode(data []byte, v any) (int, error) {
	if len(data) < 5 || data[0] != magicByte {
		return 0, errBadWireFormat
	}
	id := int(binary.BigEndian.Uint32(data[1:5]))
	sc, ok := s.schemas[id]
	if !ok {
		return id, fmt.Errorf("kafka: no schema registered for id %d", id)
	}
	return id, avro.Unmarshal(sc, data[5:], v)
}
