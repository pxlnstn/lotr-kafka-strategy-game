package engine

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// runCoordinator runs a Kafka consumer-group election. All instances join the
// "rotme-engine-leader" group on the single-partition game.session topic;
// whichever instance is assigned partition 0 is the leader and runs the
// authoritative pipeline. When that instance dies, Kafka rebalances the
// partition to a survivor, which is promoted.
func (e *Engine) runCoordinator(ctx context.Context) {
	var mu sync.Mutex
	var cancel context.CancelFunc

	becomeLeader := func() {
		mu.Lock()
		defer mu.Unlock()
		if cancel != nil {
			return
		}
		lctx, c := context.WithCancel(ctx)
		cancel = c
		log.Printf("[%s] became LEADER", e.instanceID)
		go e.runLeader(lctx)
	}
	resign := func() {
		mu.Lock()
		defer mu.Unlock()
		if cancel != nil {
			log.Printf("[%s] resigned leadership", e.instanceID)
			cancel()
			cancel = nil
		}
	}

	assigned := func(_ context.Context, _ *kgo.Client, a map[string][]int32) {
		for _, p := range a[topicSession] {
			if p == 0 {
				becomeLeader()
			}
		}
	}
	revoked := func(_ context.Context, _ *kgo.Client, r map[string][]int32) {
		for _, p := range r[topicSession] {
			if p == 0 {
				resign()
			}
		}
	}

	cl, err := kgo.NewClient(
		kgo.SeedBrokers(e.brokers...),
		kgo.ConsumerGroup("rotme-engine-leader"),
		kgo.ConsumeTopics(topicSession),
		kgo.OnPartitionsAssigned(assigned),
		kgo.OnPartitionsRevoked(revoked),
		kgo.SessionTimeout(10*time.Second),
	)
	if err != nil {
		log.Printf("[%s] coordinator: %v", e.instanceID, err)
		return
	}
	defer cl.Close()

	for {
		cl.PollFetches(ctx) // drives membership and rebalance callbacks
		if ctx.Err() != nil {
			resign()
			return
		}
	}
}
