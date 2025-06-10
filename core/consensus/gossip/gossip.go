package gossip

import (
	"context"
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	"github.com/google/uuid"
	jsoniter "github.com/json-iterator/go"
	log "github.com/sirupsen/logrus"
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
	GetDiscoverySubscribers() []warpnet.WarpPeerID
	OwnerID() string
}

type gossipConsensus struct {
	ctx         context.Context
	broadcaster ConsensusBroadcaster
	streamer    ConsensusStreamer
	recvChan    chan event.ValidationEventResponse
	isClosed    atomic.Bool
	validators  []ValidatorFunc
}

func NewGossipConsensus(
	ctx context.Context,
	broadcaster ConsensusBroadcaster,
	streamer ConsensusStreamer,
	validators ...ValidatorFunc,
) (*gossipConsensus, error) {
	return &gossipConsensus{
		ctx:         ctx,
		broadcaster: broadcaster,
		streamer:    streamer,
		validators:  validators,
		recvChan:    make(chan event.ValidationEventResponse, len(broadcaster.GetDiscoverySubscribers())),
	}, nil
}

func (g *gossipConsensus) AskValidation(data event.ValidationEvent) error {
	bt, err := json.JSON.Marshal(data)
	if err != nil {
		return err
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body:      &body,
		Path:      event.PUBLIC_GET_NODE_VALIDATE,
		NodeId:    g.broadcaster.OwnerID(),
		Timestamp: time.Now(),
		Version:   "0.0.0",
		MessageId: uuid.New().String(),
	}
	if err := g.broadcaster.PublishValidationRequest(msg); err != nil {
		return err
	}

	return g.listenResponses()
}

func (g *gossipConsensus) listenResponses() error {
	var (
		timeoutTicker  = time.NewTicker(time.Minute * 5)
		runTicker      = time.NewTicker(time.Second * 5)
		knownPeers     = make(map[string]struct{})
		peersResponded = map[string]struct{}{}
	)
	defer timeoutTicker.Stop()
	defer runTicker.Stop()
	defer g.isClosed.Store(true)
	defer close(g.recvChan)

	for range timeoutTicker.C {
		peers := g.broadcaster.GetDiscoverySubscribers()
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
			if total == 0 {
				log.Errorf("gossip consensus: timeout: no peers in the peerstore")
				return ErrConsensusRejection
			}
			log.Infof("gossip consensus: timed out waiting for peers to respond, %d peers responded, total %d", count, total)
			if float64(count)/float64(total) < quorumRatio {
				return ErrConsensusRejection
			}
			return nil
		case <-g.ctx.Done():
			return ErrConsensusRejection
		case resp := <-g.recvChan:
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
				return ErrConsensusRejection
			}
			peersResponded[resp.ValidatorID] = struct{}{}
		default:
			if total != 0 && count == total {
				return nil
			}
		}
	}
	return ErrConsensusRejection
}

// internal call from client node from PubSub
func (g *gossipConsensus) Validate(data []byte, _ warpnet.WarpStream) (any, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var ev event.ValidationEvent
	if err := json.JSON.Unmarshal(data, &ev); err != nil {
		log.Errorf("pubsub: failed to decode user update message: %v %s", err, data)
		return nil, err
	}

	var (
		result = event.Valid
		reason string
	)
	for _, validator := range g.validators {
		if err := validator(ev); err != nil {
			result = event.Invalid
			reason = err.Error()
			break
		}
	}
	return g.streamer.GenericStream(
		ev.ValidatedNodeID,
		event.PUBLIC_GET_NODE_VALIDATION_RESULT,
		event.ValidationEventResponse{
			Result:      result,
			ValidatedID: ev.ValidatedNodeID,
			Reason:      &reason,
		})
}

func (g *gossipConsensus) ValidationResult(data []byte, s warpnet.WarpStream) (any, error) {
	if g.isClosed.Load() {
		return nil, nil
	}
	if len(data) == 0 {
		return nil, nil
	}
	var resp event.ValidationEventResponse
	if err := json.JSON.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	resp.ValidatorID = s.Conn().RemotePeer().String()

	g.recvChan <- resp
	return nil, nil
}
