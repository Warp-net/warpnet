package crdt

import (
	"context"
	"errors"
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
