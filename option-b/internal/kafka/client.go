package kafka

import (
	"context"
	"fmt"

	"github.com/twmb/franz-go/pkg/kgo"
)

// Producer wraps a franz-go client for publishing Avro-encoded records.
type Producer struct {
	kc    *kgo.Client
	serde *AvroSerde
}

// NewProducer creates an idempotent producer (acks=all). franz-go enables the
// idempotent producer by default, which gives us no-duplicate, in-order writes.
func NewProducer(brokers []string, serde *AvroSerde) (*Producer, error) {
	kc, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.ProducerBatchMaxBytes(1_000_000),
	)
	if err != nil {
		return nil, err
	}
	return &Producer{kc: kc, serde: serde}, nil
}

// NewTxnProducer creates a transactional producer for exactly-once output
// (used for the GameOver event).
func NewTxnProducer(brokers []string, txnID string, serde *AvroSerde) (*Producer, error) {
	kc, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
		kgo.TransactionalID(txnID),
	)
	if err != nil {
		return nil, err
	}
	return &Producer{kc: kc, serde: serde}, nil
}

// Produce Avro-encodes val under schemaID and writes it synchronously.
func (p *Producer) Produce(ctx context.Context, topic, key string, schemaID int, val any) error {
	value, err := p.serde.Encode(schemaID, val)
	if err != nil {
		return err
	}
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: value}
	return p.kc.ProduceSync(ctx, rec).FirstErr()
}

// ProduceInTxn writes one record inside a transaction and commits it. If
// anything fails the transaction is aborted, so the record never becomes
// visible. This is what makes GameOver appear exactly once.
func (p *Producer) ProduceInTxn(ctx context.Context, topic, key string, schemaID int, val any) error {
	value, err := p.serde.Encode(schemaID, val)
	if err != nil {
		return err
	}
	if err := p.kc.BeginTransaction(); err != nil {
		return err
	}
	rec := &kgo.Record{Topic: topic, Key: []byte(key), Value: value}
	if err := p.kc.ProduceSync(ctx, rec).FirstErr(); err != nil {
		_ = p.kc.EndTransaction(ctx, kgo.TryAbort)
		return err
	}
	return p.kc.EndTransaction(ctx, kgo.TryCommit)
}

func (p *Producer) Close() { p.kc.Close() }

// Consumer wraps a franz-go consumer-group client.
type Consumer struct {
	kc    *kgo.Client
	serde *AvroSerde
}

// NewConsumer joins the given consumer group and subscribes to topics. All
// instances sharing a group split the partitions between them; if one dies,
// Kafka rebalances its partitions to the survivors.
func NewConsumer(brokers []string, group string, topics []string, serde *AvroSerde) (*Consumer, error) {
	kc, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumerGroup(group),
		kgo.ConsumeTopics(topics...),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		kgo.AutoCommitMarks(),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{kc: kc, serde: serde}, nil
}

// Record is a decoded message handed to callers.
type Record struct {
	Topic     string
	Key       string
	Value     []byte
	Partition int32
	Offset    int64
}

// Poll blocks until records arrive or ctx is cancelled, returning the raw
// records. Callers decode with the serde as needed.
func (c *Consumer) Poll(ctx context.Context) ([]Record, error) {
	fetches := c.kc.PollFetches(ctx)
	if errs := fetches.Errors(); len(errs) > 0 {
		return nil, fmt.Errorf("poll: %v", errs[0].Err)
	}
	var out []Record
	fetches.EachRecord(func(r *kgo.Record) {
		out = append(out, Record{
			Topic: r.Topic, Key: string(r.Key), Value: r.Value,
			Partition: r.Partition, Offset: r.Offset,
		})
		c.kc.MarkCommitRecords(r)
	})
	return out, nil
}

func (c *Consumer) Decode(value []byte, v any) (int, error) { return c.serde.Decode(value, v) }

func (c *Consumer) Close() { c.kc.Close() }
