package isolation

import (
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	log "github.com/sirupsen/logrus"
)

type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, body any) (err error)
}

type StreamingNode interface {
	GenericStream(string, stream.WarpRoute, any) ([]byte, error)
	Node() warpnet.P2PNode
}

type IsolationProtocol struct {
	pub  Publisher
	node StreamingNode
}

func NewIsolationProtocol(node StreamingNode, pub Publisher) *IsolationProtocol {
	return &IsolationProtocol{pub: pub, node: node}
}

func (ip *IsolationProtocol) IsolateTweet(peerId warpnet.WarpPeerID, t *domain.Tweet, m *domain.TweetModeration) {
	if m == nil {
		return
	}

	result := event.ModerationResultEvent{
		Type:     domain.ModerationTweetType,
		UserID:   t.UserId,
		ObjectID: &t.Id,
		Reason:   m.Reason,
		Model:    domain.LLAMA2,
		Result:   m.IsOk,
	}

	_, err := ip.node.GenericStream(
		peerId.String(),
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	)
	if err != nil {
		log.Errorf("moderator: post moderation result: %v", err)
	}

	if err := ip.pub.PublishUpdateToFollowers(
		t.UserId,
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	); err != nil {
		log.Errorf("broadcaster publish owner tweet update: %v", err)
	}
}
