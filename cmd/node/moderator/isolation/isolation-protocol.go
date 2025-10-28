package isolation

import (
	"github.com/Warp-net/warpnet/core/stream"
	"github.com/Warp-net/warpnet/core/warpnet"
	"github.com/Warp-net/warpnet/domain"
	"github.com/Warp-net/warpnet/event"
	"github.com/Warp-net/warpnet/json"
	log "github.com/sirupsen/logrus"
)

type Publisher interface {
	PublishUpdateToFollowers(ownerId, dest string, bt []byte) (err error)
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

func (ip *IsolationProtocol) IsolateTweet(nodeId warpnet.WarpPeerID, tweet domain.Tweet) {
	bt, _ := json.Marshal(tweet)
	if err := ip.pub.PublishUpdateToFollowers(tweet.UserId, event.PRIVATE_POST_TWEET, bt); err != nil {
		log.Errorf("broadcaster publish owner tweet update: %v", err)
	}

	var (
		resultType = domain.OK
		reason     *string
	)
	if tweet.Moderation != nil && tweet.Moderation.IsOk {
		resultType = domain.FAIL
		reason = tweet.Moderation.Reason
	}

	result := event.ModerationResultEvent{
		Type:     domain.ModerationTweetType,
		NodeID:   ip.node.Node().ID().String(),
		UserID:   tweet.UserId,
		ObjectID: &tweet.Id,
		Reason:   reason,
		Result:   resultType,
	}
	result.ObjectID = &tweet.Id
	result.Reason = reason
	result.Result = domain.FAIL

	_, err := ip.node.GenericStream(
		nodeId.String(),
		event.PUBLIC_POST_MODERATION_RESULT,
		result,
	)
	if err != nil {
		log.Errorf("moderator: post moderation result: %v", err)
	}
}
