package consensus

import (
	"context"
	"errors"
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
	ctx                         context.Context
	broadcaster                 ConsensusBroadcaster
	streamer                    ConsensusStreamer
	recvChan                    chan event.ValidationResultEvent
	isClosed, isBg, isValidated atomic.Bool
	interruptChan               chan os.Signal
	validators                  []ValidatorFunc
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
		timeout       = time.Minute * 50
		timeoutTicker = time.NewTicker(timeout)
		runTicker     = time.NewTicker(time.Minute)
		//knownPeers     = make(map[string]struct{})
		validResponses = map[string]struct{}{}
	)
	defer timeoutTicker.Stop()
	defer runTicker.Stop()

	for range runTicker.C {
		if g.isClosed.Load() {
			return
		}
		if g.isValidated.Load() {
			return
		}
		peers := g.broadcaster.GetConsensusTopicSubscribers()
		if isMeAlone(peers, g.broadcaster.OwnerID()) {
			timeoutTicker.Reset(timeout)
			continue
		}
		//for _, id := range peers {
		//	knownPeers[id.String()] = struct{}{}
		//}
		var (
			total = len(peers)
			count = len(validResponses)
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
			log.Infoln("gossip consensus: consensus successful!")
			g.isValidated.Store(true)
			return
		case <-g.ctx.Done():
			log.Infoln("gossip consensus: listen: context done")
			return
		case resp, ok := <-g.recvChan:
			log.Infoln("gossip consensus: listener: validation response received", resp.ValidatorID)
			if !ok {
				log.Warningln("gossip consensus: listener: channel closed")
				return
			}
			//if _, isKnown := knownPeers[resp.ValidatorID]; !isKnown {
			//	log.Infof("gossip consensus: listener: node %s is unknown", resp.ValidatorID)
			//	continue
			//}
			if _, isSeen := validResponses[resp.ValidatorID]; isSeen {
				log.Infof("gossip consensus: listener: node %s is already participated", resp.ValidatorID)
				continue
			}
			if resp.Result == event.Invalid {
				if resp.Reason != nil {
					log.Errorf(
						"gossip consensus: listener: validator responded with 'invalid node' result: %s", *resp.Reason,
					)
				}
				continue
			}
			fmt.Println()
			log.Infoln("gossip consensus: validator responded with 'valid node' result", resp.ValidatorID)
			fmt.Println()
			validResponses[resp.ValidatorID] = struct{}{}
		default:
			if total != 0 && count == total {
				g.isValidated.Store(true)
				log.Infoln("gossip consensus: consensus successful!")
				return
			}
		}
	}
}

func (g *gossipConsensus) AskValidation(data event.ValidationEvent) error {
	if g.isValidated.Load() {
		return nil
	}
	if g.isBg.Load() {
		return errors.New("gossip consensus: ask validation: already in progress")
	}

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
		Version:   "0.0.0", // TODO manage protocol versions properly
		MessageId: uuid.New().String(),
	}

	peers := g.broadcaster.GetConsensusTopicSubscribers()
	if !isMeAlone(peers, g.broadcaster.OwnerID()) {
		if err := g.broadcaster.PublishValidationRequest(msg); err != nil {
			return err
		}
	}
	log.Infoln("gossip consensus: node is alone - go to background validation")
	go g.runBackgroundValidation(msg)
	g.isBg.Store(true)
	return nil
}

func (g *gossipConsensus) runBackgroundValidation(msg event.Message) {
	defer g.isBg.Store(false)

	t := time.NewTicker(time.Minute)
	defer t.Stop()

	for {
		if g.isClosed.Load() {
			log.Infoln("gossip consensus: node is closed - stop background validation")
			return
		}
		if g.isValidated.Load() {
			log.Infoln("gossip consensus: node is validated - stop background validation")
			return
		}

		select {
		case <-g.ctx.Done():
			log.Infoln("gossip consensus: background: context done")
			return
		case <-t.C:
			peers := g.broadcaster.GetConsensusTopicSubscribers()
			if isMeAlone(peers, g.broadcaster.OwnerID()) {
				log.Infoln("gossip consensus: node is alone")
				continue
			}

			msg.Timestamp = time.Now()
			msg.MessageId = uuid.New().String()

			if err := g.broadcaster.PublishValidationRequest(msg); err != nil {
				log.Errorf("gossip consensus: ask validation: %v", err)
				g.interruptChan <- os.Interrupt
				return
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

	log.Infof("gossip consensus: sending validation result to: %s", ev.ValidatedNodeID)
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
		log.Infoln("gossip consensus: closed")
		return errors.New("gossip consensus: closed")
	}

	g.recvChan <- ev
	return nil
}

func (g *gossipConsensus) Close() {
	g.isClosed.Store(true)
	close(g.recvChan)
}
