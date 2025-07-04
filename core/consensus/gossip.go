package consensus

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

const quorumRatio float64 = 0.75

type ConsensusStreamer interface {
	GenericStream(nodeId string, path stream.WarpRoute, data any) (_ []byte, err error)
	NodeInfo() warpnet.NodeInfo
	Peerstore() warpnet.WarpPeerstore
}

type ValidatorFunc func(data event.ValidationEvent) error

type ConsensusHandler func(message *event.Message)

type ConsensusBroadcaster interface {
	PublishValidationRequest(msg event.Message) (err error)
	GetConsensusTopicSubscribers() []warpnet.WarpAddrInfo
	SubscribeConsensusTopic() error
	OwnerID() string
}

type gossipConsensus struct {
	ctx                                     context.Context
	broadcaster                             ConsensusBroadcaster
	streamer                                ConsensusStreamer
	recvChan                                chan event.ValidationResultEvent
	isClosed, isBgRunning, isValidationDone atomic.Bool
	interruptChan                           chan os.Signal
	validators                              []ValidatorFunc
}

func NewGossipConsensus(
	ctx context.Context,
	broadcaster ConsensusBroadcaster,
	interruptChan chan os.Signal,
	validators ...ValidatorFunc,
) *gossipConsensus {
	gc := &gossipConsensus{
		ctx:           ctx,
		broadcaster:   broadcaster,
		validators:    validators,
		interruptChan: interruptChan,
		recvChan:      make(chan event.ValidationResultEvent),
	}
	return gc
}

func (g *gossipConsensus) Start(streamer ConsensusStreamer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v %v", r, debug.Stack())
		}
	}()

	g.streamer = streamer

	if err := g.broadcaster.SubscribeConsensusTopic(); err != nil {
		return err
	}

	subsNum := len(g.broadcaster.GetConsensusTopicSubscribers())
	if subsNum == 0 {
		subsNum = 1
	}
	g.recvChan = make(chan event.ValidationResultEvent, subsNum)

	log.Infoln("gossip consensus: started")

	go g.listenResponses()
	return nil
}

func (g *gossipConsensus) listenResponses() {
	var (
		timeout          = time.Minute * 5
		timeoutTicker    = time.NewTicker(timeout)
		ticker           = time.NewTicker(time.Second)
		knownPeers       = map[string]struct{}{}
		validResponses   = map[string]struct{}{}
		invalidResponses = map[string]struct{}{}
	)
	defer timeoutTicker.Stop()
	defer ticker.Stop()
	defer func() {
		g.isValidationDone.Store(true)
		log.Infoln("gossip consensus: listener exited")
	}()

	failNowF := func() {
		select {
		case g.interruptChan <- os.Interrupt:
		default:
		}
	}

	for range ticker.C {
		if g.isClosed.Load() {
			return
		}
		if g.isValidationDone.Load() {
			return
		}

		peers := g.streamer.Peerstore().Peers()
		for _, id := range peers {
			knownPeers[id.String()] = struct{}{}
		}
		subscribers := g.broadcaster.GetConsensusTopicSubscribers()
		if isMeAlone(subscribers, g.broadcaster.OwnerID()) {
			timeoutTicker.Reset(timeout)
			continue
		}
		var (
			total        = len(subscribers)
			validCount   = len(validResponses)
			invalidCount = len(invalidResponses)
		)

		log.Infof(
			"gossip consensus: validation in progess: valid [%d], invalid [%d], total [%d]",
			validCount, invalidCount, total,
		)

		select {
		case <-timeoutTicker.C:
			if validCount == 0 {
				log.Errorf("gossip consensus: timeout: no peers responded")
				failNowF()
				return
			}

			ratio := float64(validCount) / float64(total)
			log.Infof("gossip consensus: timed out: ratio: %f", ratio)
			if ratio < quorumRatio {
				failNowF()
				return
			}
			log.Infoln("gossip consensus: consensus successful!")
			return
		case <-g.ctx.Done():
			log.Infoln("gossip consensus: listen: context done")
			return
		case resp, ok := <-g.recvChan:
			if !ok {
				log.Warningln("gossip consensus: listener: channel closed")
				return
			}
			if _, isKnown := knownPeers[resp.ValidatorID]; !isKnown {
				log.Warnf("gossip consensus: listener: node %s is unknown", resp.ValidatorID)
				continue
			}
			switch resp.Result {
			case event.Valid:
				if total != 0 && validCount == total {
					log.Infoln("gossip consensus: consensus successful!")
					return
				}

				if _, isSeen := validResponses[resp.ValidatorID]; isSeen {
					continue
				}
				log.Infoln("gossip consensus: validator responded with 'valid' result", resp.ValidatorID)
				validResponses[resp.ValidatorID] = struct{}{}

			case event.Invalid:
				if total != 0 && invalidCount == total {
					log.Infoln("gossip consensus: consensus failed!")
					failNowF()
					return
				}
				if _, isSeen := invalidResponses[resp.ValidatorID]; isSeen {
					continue
				}
				if resp.Reason != nil {
					log.Errorf("gossip consensus: listener: 'invalid node' result: %s", *resp.Reason)
				}
				invalidResponses[resp.ValidatorID] = struct{}{}
			}
		}
	}
}

func (g *gossipConsensus) AskValidation(data event.ValidationEvent) {
	if g.isValidationDone.Load() {
		return
	}
	if g.isBgRunning.Load() {
		return
	}

	bt, err := json.JSON.Marshal(data)
	if err != nil {
		log.Errorf("gossip consensus: failed to marshal validation event: %s", err)
		g.interruptChan <- os.Interrupt
		return
	}
	body := jsoniter.RawMessage(bt)

	msg := event.Message{
		Body:      &body,
		Path:      event.INTERNAL_POST_NODE_VALIDATE,
		NodeId:    g.broadcaster.OwnerID(),
		Timestamp: time.Now(),
		Version:   "0.0.0", // TODO manage protocol versions properly
		MessageId: uuid.New().String(),
	}

	g.runBackgroundPublishing(msg)
	return
}

func (g *gossipConsensus) runBackgroundPublishing(msg event.Message) {
	g.isBgRunning.Store(true)
	defer func() {
		g.isBgRunning.Store(false)
		log.Infoln("gossip consensus: background publishing exited")
	}()

	t := time.NewTicker(time.Second * 2)
	defer t.Stop()

	for {
		if g.isClosed.Load() {
			log.Infoln("gossip consensus: node is closed - stop background validation")
			return
		}
		if g.isValidationDone.Load() {
			log.Infoln("gossip consensus: node validation finished - stop background publishing")
			return
		}

		select {
		case <-g.ctx.Done():
			log.Infoln("gossip consensus: background: context done")
			return
		case <-t.C:
			peers := g.broadcaster.GetConsensusTopicSubscribers()
			if isMeAlone(peers, g.broadcaster.OwnerID()) {
				continue
			}

			msg.Timestamp = time.Now()
			msg.MessageId = uuid.New().String()

			if err := g.broadcaster.PublishValidationRequest(msg); err != nil {
				log.Errorf("gossip consensus: ask validation: %v", err)
			}
		}
	}
}

func isMeAlone(peers []warpnet.WarpAddrInfo, ownerId string) bool {
	return len(peers) == 0 || (len(peers) == 1 && peers[0].ID.String() == ownerId)
}

// Validate is internal call from client node and from PubSub
func (g *gossipConsensus) Validate(ev event.ValidationEvent) error {
	var (
		result = event.Valid
		reason string
	)
	for _, validator := range g.validators {
		if err := validator(ev); err != nil {
			log.Errorf("gossip consensus: validator applied: %s", err.Error())
			result = event.Invalid
			reason = err.Error()
			break
		}
	}

	_, err := g.streamer.GenericStream(
		ev.ValidatedNodeID,
		event.PUBLIC_POST_NODE_VALIDATION_RESULT,
		event.ValidationResultEvent{
			Result:      result,
			ValidatedID: ev.ValidatedNodeID,
			Reason:      &reason,
		})
	return err
}

func (g *gossipConsensus) ValidationResult(ev event.ValidationResultEvent) error {
	if g.isClosed.Load() {
		return nil
	}
	if g.isValidationDone.Load() {
		return nil
	}
	g.recvChan <- ev
	return nil
}

func (g *gossipConsensus) Close() {
	g.isClosed.Store(true)
	close(g.recvChan)
}
