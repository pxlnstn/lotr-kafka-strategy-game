// Command smoke is a manual diagnostic: it produces one Avro-encoded order to
// game.orders.raw and reads it back, proving the serde, registry, and Kafka
// client work end to end against the running cluster. Not part of the test
// suite (it needs Kafka + Schema Registry up).
//
// Run from the host:  go run ./cmd/smoke
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"rotme/internal/kafka"
)

type orderSubmitted struct {
	PlayerID  string `avro:"playerId"`
	UnitID    string `avro:"unitId"`
	OrderType string `avro:"orderType"`
	Payload   []byte `avro:"payload"`
	Turn      int    `avro:"turn"`
	Timestamp int64  `avro:"timestamp"`
}

func main() {
	brokers := []string{"localhost:9092"}
	serde := kafka.NewAvroSerde()
	reg := kafka.NewRegistry("http://localhost:8081")

	ids, err := reg.LoadSchemas(serde, map[string]string{
		"OrderSubmitted": "game.orders.raw-value",
	})
	if err != nil {
		log.Fatalf("load schemas: %v", err)
	}
	id := ids["OrderSubmitted"]
	fmt.Printf("OrderSubmitted schema id = %d\n", id)

	prod, err := kafka.NewProducer(brokers, serde)
	if err != nil {
		log.Fatal(err)
	}
	defer prod.Close()

	msg := orderSubmitted{
		PlayerID: "light", UnitID: "ring-bearer", OrderType: "ASSIGN_ROUTE",
		Payload: []byte(`{"pathIds":["shire-to-bree"]}`), Turn: 1,
		Timestamp: time.Now().UnixMilli(),
	}
	ctx := context.Background()
	if err := prod.Produce(ctx, "game.orders.raw", msg.PlayerID, id, msg); err != nil {
		log.Fatalf("produce: %v", err)
	}
	fmt.Println("produced one order to game.orders.raw")

	group := fmt.Sprintf("smoke-%d", time.Now().UnixNano())
	cons, err := kafka.NewConsumer(brokers, group, []string{"game.orders.raw"}, serde)
	if err != nil {
		log.Fatal(err)
	}
	defer cons.Close()

	pollCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		recs, err := cons.Poll(pollCtx)
		if err != nil {
			log.Fatalf("poll: %v", err)
		}
		for _, r := range recs {
			var got orderSubmitted
			sid, err := cons.Decode(r.Value, &got)
			if err != nil {
				log.Fatalf("decode: %v", err)
			}
			fmt.Printf("consumed (schema id %d): unit=%s order=%s turn=%d payload=%s\n",
				sid, got.UnitID, got.OrderType, got.Turn, got.Payload)
			return
		}
	}
}
