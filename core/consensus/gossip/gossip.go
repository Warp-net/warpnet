package gossip

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	jsoniter "github.com/json-iterator/go"
)

const consensusAnnounsementsResponsePrefix = "consensus-announcements-%s"

type ConsensusHandler func(message *event.Message)

type ConsensusBroadcaster interface {
	GenericSubscribe(topics ...string) (err error)
	PublishConsensusAnnouncement(msg event.Message) (err error)
	GetDiscoverySubscribers() []warpnet.WarpPeerID
	OwnerID() string
}

type gossipConsensus struct {
	ctx               context.Context
	broadcaster       ConsensusBroadcaster
	responseTopicName string
	recvChan          chan event.ValidationEventResponse
}

func NewGossipConsensus(
	ctx context.Context,
	broadcaster ConsensusBroadcaster,
) (*gossipConsensus, error) {
	topicName := fmt.Sprintf(consensusAnnounsementsResponsePrefix, broadcaster.OwnerID())
	if err := broadcaster.GenericSubscribe(topicName); err != nil {
		return nil, err
	}
	return &gossipConsensus{
		ctx:               ctx,
		broadcaster:       broadcaster,
		responseTopicName: topicName,
		recvChan:          make(chan event.Message, len(broadcaster.GetDiscoverySubscribers())),
	}, nil
}

func (g *gossipConsensus) AskValidation(data event.ValidationEvent) error {
	bt, err := json.JSON.Marshal(data)
	if err != nil {
		return err
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body: &body,
		Path: g.responseTopicName,
	}
	if err := g.broadcaster.PublishConsensusAnnouncement(msg); err != nil {
		return err
	}

	count := 0
	for {
		allPeersNum := len(g.broadcaster.GetDiscoverySubscribers())

		select {
		case <-g.ctx.Done():
			return nil
		case resp := <-g.recvChan:

		}

		if count == allPeersNum {
			return nil
		}
	}
}

func (g *gossipConsensus) ValidationResponseHandler(msg event.Message) error {
	if g == nil {
		return nil
	}
	if msg.Body == nil {
		return nil
	}
	var resp event.ValidationEventResponse
	if err := json.JSON.Unmarshal(*msg.Body, &resp); err != nil {
		return err
	}
	resp.UniqueID = msg.NodeId

	g.recvChan <- resp
	return nil
}
