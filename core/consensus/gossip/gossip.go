package gossip

import (
	"context"
	"fmt"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
	"os"
	"runtime/debug"
	"sync/atomic"
	"time"
)

const (
	ErrConsensusRejection         = warpnet.WarpError("request rejected by consensus")
	quorumRatio           float64 = 0.75
)

type ConsensusStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
}

type ValidatorFunc func(data event.ValidationEvent) error

type ConsensusHandler func(message *event.Message)

type ConsensusBroadcaster interface {
	GenericSubscribe(topics ...string) (err error)
	PublishValidationRequest(msg event.Message) (err error)
	GetConsensusTopicSubscribers() []warpnet.WarpPeerID
	OwnerID() string
}

type gossipConsensus struct {
	ctx           context.Context
	broadcaster   ConsensusBroadcaster
	streamer      ConsensusStreamer
	recvChan      chan event.ValidationEventResponse
	isClosed      atomic.Bool
	interruptChan chan os.Signal
	validators    []ValidatorFunc
}

func NewGossipConsensus(
	ctx context.Context,
	broadcaster ConsensusBroadcaster,
	streamer ConsensusStreamer,
	interruptChan chan os.Signal,
	validators ...ValidatorFunc,
) *gossipConsensus {
	gc := &gossipConsensus{
		ctx:           ctx,
		broadcaster:   broadcaster,
		streamer:      streamer,
		validators:    validators,
		interruptChan: interruptChan,
		recvChan:      make(chan event.ValidationEventResponse, len(broadcaster.GetConsensusTopicSubscribers())),
	}
	return gc
}

func (g *gossipConsensus) Start(data event.ValidationEvent) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v %v", r, debug.Stack())
		}
	}()
	log.Infoln("gossip consensus: started")

	go g.listenResponses()

	peers := g.broadcaster.GetConsensusTopicSubscribers()
	if !isMeAlone(peers, g.broadcaster.OwnerID()) {
		if err := g.AskValidation(data); err != nil {
			return err
		}
	}
	log.Infoln("gossip consensus: node is alone - go to background validation")
	go g.runBackgroundValidation(data)
	return nil
}

func (g *gossipConsensus) listenResponses() {
	var (
		timeoutTicker  = time.NewTicker(time.Minute)
		runTicker      = time.NewTicker(time.Second * 5)
		knownPeers     = make(map[string]struct{})
		peersResponded = map[string]struct{}{}
	)
	defer timeoutTicker.Stop()
	defer runTicker.Stop()

	for range runTicker.C {
		if g.isClosed.Load() {
			return
		}
		peers := g.broadcaster.GetConsensusTopicSubscribers()
		if isMeAlone(peers, g.broadcaster.OwnerID()) {
			timeoutTicker.Reset(time.Minute)
			continue
		}
		for _, id := range peers {
			knownPeers[id.String()] = struct{}{}
		}
		var (
			total = len(peers)
			count = len(peersResponded)
		)

		log.Infof("gossip consensus: validation in progess: %d/%d", count, total)

		select {
		case <-timeoutTicker.C:
			if count == 0 {
				log.Errorf("gossip consensus: timeout: no peers responded")
				g.interruptChan <- os.Interrupt
				return
			}
			log.Infof("gossip consensus: timed out waiting for peers to respond, %d peers responded, total %d", count, total)
			if float64(count)/float64(total) < quorumRatio {
				g.interruptChan <- os.Interrupt
				return
			}
			return
		case <-g.ctx.Done():
			return
		case resp, ok := <-g.recvChan:
			if !ok {
				return
			}
			if _, isKnown := knownPeers[resp.ValidatorID]; !isKnown {
				continue
			}
			if _, isSeen := peersResponded[resp.ValidatorID]; isSeen {
				continue
			}
			if resp.Result == event.Invalid {
				if resp.Reason != nil {
					log.Errorf("gossip consensus: validator responded with 'invalid node' result: %s", *resp.Reason)
				}
				g.interruptChan <- os.Interrupt
				return
			}
			peersResponded[resp.ValidatorID] = struct{}{}
		default:
			if total != 0 && count == total {
				return
			}
		}
	}
}

func (g *gossipConsensus) runBackgroundValidation(data event.ValidationEvent) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()

	for {
		if g.isClosed.Load() {
			return
		}
		select {
		case <-g.ctx.Done():
			return
		case <-t.C:
			peers := g.broadcaster.GetConsensusTopicSubscribers()
			if isMeAlone(peers, g.broadcaster.OwnerID()) {
				continue
			}
			if err := g.AskValidation(data); err != nil {
				g.interruptChan <- os.Interrupt
				return
			}
		}
	}
}

func isMeAlone(peers []warpnet.WarpPeerID, ownerId string) bool {
	return len(peers) == 0 || (len(peers) == 1 && peers[0].String() == ownerId)
}

func (g *gossipConsensus) AskValidation(data event.ValidationEvent) error {
	bt, err := json.JSON.Marshal(data)
	if err != nil {
		return err
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body:      &body,
		Path:      event.PRIVATE_POST_NODE_VALIDATE,
		NodeId:    g.broadcaster.OwnerID(),
		Timestamp: time.Now(),
		Version:   "0.0.0", // TODO
		MessageId: uuid.New().String(),
	}

	return g.broadcaster.PublishValidationRequest(msg)
}

// Validate is internal call from client node and from PubSub
func (g *gossipConsensus) Validate(data []byte, s warpnet.WarpStream) (any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var ev event.ValidationEvent
	if err := json.JSON.Unmarshal(data, &ev); err != nil {
		log.Errorf("pubsub: failed to decode user update message: %v %s", err, data)
		return nil, err
	}
	if ev.ValidatedNodeID == s.Conn().LocalPeer().String() { // no need to validate self
		return nil, nil
	}

	var (
		result = event.Valid
		reason string
	)
	for _, validator := range g.validators {
		if err := validator(ev); err != nil {
			log.Errorf("gossip consensus: validation failed: %s", data)

			result = event.Invalid
			reason = err.Error()
			break
		}
	}

	log.Infof("gossip consensus: sending validation result to: %s", ev.ValidatedNodeID)
	bt, err := g.streamer.GenericStream(
		ev.ValidatedNodeID,
		event.PUBLIC_POST_NODE_VALIDATION_RESULT,
		event.ValidationEventResponse{
			Result:      result,
			ValidatedID: ev.ValidatedNodeID,
			Reason:      &reason,
		})
	if err != nil {
		log.Errorf("gossip consensus: failed to send validation result: %v", err)
	}
	return bt, err
}

func (g *gossipConsensus) ValidationResult(data []byte, s warpnet.WarpStream) (any, error) {
	log.Infof("gossip consensus: validation result received: %s", data)
	if g.isClosed.Load() {
		log.Infoln("gossip consensus: closed")
		return nil, nil
	}
	if len(data) == 0 {
		return nil, nil
	}

	var resp event.ValidationEventResponse
	if err := json.JSON.Unmarshal(data, &resp); err != nil {
		log.Errorf("gossip consensus: failed to decode validation result: %v %s", err, data)
		return nil, err
	}

	resp.ValidatorID = s.Conn().RemotePeer().String()
	if resp.ValidatorID == s.Conn().LocalPeer().String() { // no need to validate self
		return nil, nil
	}

	g.recvChan <- resp
	return nil, nil
}

func (g *gossipConsensus) Close() {
	g.isClosed.Store(true)
	close(g.recvChan)
}
