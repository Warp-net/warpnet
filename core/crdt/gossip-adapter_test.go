package crdt

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockGossipPubSub struct {
	published []struct {
		topic string
		data  []byte
	}
	subscribeHandler func([]byte) error
	publishErr       error
	subscribeErr     error
}

func (m *mockGossipPubSub) PublishRaw(topicName string, data []byte) error {
	if m.publishErr != nil {
		return m.publishErr
	}
	m.published = append(m.published, struct {
		topic string
		data  []byte
	}{topicName, data})
	return nil
}

func (m *mockGossipPubSub) SubscribeRaw(topicName string, h func([]byte) error) error {
	if m.subscribeErr != nil {
		return m.subscribeErr
	}
	m.subscribeHandler = h
	return nil
}

func TestNewGossipBroadcaster(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, err := NewGossipBroadcaster(ctx, mock)
	assert.NoError(t, err)
	assert.NotNil(t, gb)
	assert.Equal(t, statsTopic, gb.topic)
}

func TestNewGossipBroadcaster_SubscribeError(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{subscribeErr: errors.New("subscribe failed")}
	_, err := NewGossipBroadcaster(ctx, mock)
	assert.Error(t, err)
}

func TestBroadcast(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	err := gb.Broadcast(ctx, []byte("hello"))
	assert.NoError(t, err)
	assert.Len(t, mock.published, 1)
	assert.Equal(t, statsTopic, mock.published[0].topic)
	assert.Equal(t, []byte("hello"), mock.published[0].data)
}

func TestBroadcast_Error(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{publishErr: errors.New("publish failed")}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	err := gb.Broadcast(ctx, []byte("data"))
	assert.Error(t, err)
}

func TestReceiveAndNext(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	gb.Receive([]byte("message1"))

	data, err := gb.Next(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []byte("message1"), data)
}

func TestNext_ContextCancelled(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	nextCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := gb.Next(nextCtx)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestNext_ParentContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	cancel()
	// Give a short delay for the goroutine to notice
	time.Sleep(10 * time.Millisecond)

	_, err := gb.Next(context.Background())
	assert.Error(t, err)
}

func TestReceive_ChannelFull_DropsOldest(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	// Fill channel to capacity
	for i := range 100 {
		gb.dataChan <- []byte{byte(i)}
	}

	// Receive should drop the oldest and add new
	gb.Receive([]byte("newest"))

	// Channel should still have 100 items
	assert.Equal(t, 100, len(gb.dataChan))
}

// TestReceive_ConcurrentWithNext_NoDeadlock is a regression test for
// the deadlock that existed when Receive() did a blocking
// `<-gb.dataChan` under the held mutex while a concurrent Next()
// drained the channel. With many producers and one consumer running
// in parallel, the producers must finish within the timeout — no
// goroutine should be parked on a channel op under gb.mx.
func TestReceive_ConcurrentWithNext_NoDeadlock(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	// Consumer blocks on Next() until either data arrives or its
	// context is cancelled — no busy-wait, no per-iter context churn.
	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for {
			if _, err := gb.Next(consumerCtx); err != nil {
				return
			}
		}
	}()

	const producers = 8
	const perProducer = 2000
	var wg sync.WaitGroup
	wg.Add(producers)
	for i := range producers {
		go func(id int) {
			defer wg.Done()
			for j := range perProducer {
				gb.Receive([]byte{byte(id), byte(j)})
			}
		}(i)
	}

	producersDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(producersDone)
	}()

	select {
	case <-producersDone:
	case <-time.After(5 * time.Second):
		stopConsumer()
		t.Fatal("Receive deadlocked: producers did not finish within 5s")
	}
	stopConsumer()
	<-consumerDone
}

func TestSubscribeHandler_ReceivesData(t *testing.T) {
	ctx := context.Background()
	mock := &mockGossipPubSub{}
	gb, _ := NewGossipBroadcaster(ctx, mock)

	// Simulate data coming through subscription
	assert.NotNil(t, mock.subscribeHandler)
	err := mock.subscribeHandler([]byte("subscribed data"))
	assert.NoError(t, err)

	data, err := gb.Next(ctx)
	assert.NoError(t, err)
	assert.Equal(t, []byte("subscribed data"), data)
}
