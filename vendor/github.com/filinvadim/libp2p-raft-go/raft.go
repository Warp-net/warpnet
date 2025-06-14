package raft

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"math/rand"
	"sync"
	"time"

	"github.com/hashicorp/raft"
)

const MaxVotersNum = 7

var ErrNoRaftCluster = errors.New("consensus: no cluster found")

type (
	ServerID        = raft.ServerID
	Server          = raft.Server
	ServerAddress   = raft.ServerAddress
	RaftStableStore = raft.StableStore
	LogStore        = raft.LogStore
	RaftConfig      = raft.Config
)

type StableStorer interface {
	RaftStableStore
	SnapshotsFilesPath() string
}

type NodeTransporter interface {
	host.Host
}

type FSMachiner interface {
	raft.FSM
	GetCurrentState() *KVState
}

type Transporter interface {
	raft.Transport
	Close() error
}

type Config struct {
	// HeartbeatTimeout specifies the time in follower state without contact
	// from a leader before we attempt an election.
	// Default: 5 sec
	HeartbeatTimeout time.Duration
	// CommitTimeout specifies the time without an Apply operation before the
	// leader sends an AppendEntry RPC to followers, to ensure a timely commit of
	// log entries.
	// Due to random staggering, may be delayed as much as 2x this value.
	// Default: 1 sec
	CommitTimeout time.Duration
	// SnapshotInterval controls how often we check if we should perform a
	// snapshot. We randomly stagger between this value and 2x this value to avoid
	// the entire cluster from performing a snapshot at once. The value passed
	// here is the initial setting used. This can be tuned during operation using
	// ReloadConfig.
	// Default: 20 min
	SnapshotInterval time.Duration
	// MaxAppendEntries controls the maximum number of append entries
	// to send at once. We want to strike a balance between efficiency
	// and avoiding waste if the follower is going to reject because of
	// an inconsistent log.
	// Default: 128
	MaxAppendEntries uint64
	// TrailingLogs controls how many logs we leave after a snapshot. This is used
	// so that we can quickly replay logs on a follower instead of being forced to
	// send an entire snapshot. The value passed here is the initial setting used.
	// This can be tuned during operation using ReloadConfig.
	// Default: 256
	TrailingLogs uint64
	// SnapshotInterval controls how often we check if we should perform a
	// snapshot. We randomly stagger between this value and 2x this value to avoid
	// the entire cluster from performing a snapshot at once. The value passed
	// here is the initial setting used. This can be tuned during operation using
	// ReloadConfig.
	// Default: 8192
	SnapshotThreshold uint64
	// FSM (finite state machine) is a crucial component used to maintain a consistent
	// state across a distributed system of nodes. Raft uses a consensus algorithm to
	// ensure all nodes in a cluster agree on the same sequence of state transitions,
	// which is managed by the FSM.
	// Default: NewFSM.
	FSM FSMachiner
	// Custom Marshaller: by default MessagePack (msgpack) is frequently used with Raft,
	// a consensus algorithm, because of its efficiency as a binary serialization format,
	// which is crucial for fast and reliable message passing in distributed systems.
	// Raft nodes exchange messages that represent changes to the system's state, and
	// MessagePack's compact nature and speed make it well-suited for this purpose
	// Default: MessagePack V5
	Codec EncodeDecoder
	// StableStore is used to provide stable storage
	// of key configurations to ensure safety.
	// Default: in-memory store
	StableStore StableStorer
	// LogStore is used as db for storing
	// and retrieving logs in a durable fashion.
	// Default: in-memory store
	LogStore LogStore
	// Cluster predefined nodes
	// Default: in-memory store
	BootstrapNodes []peer.AddrInfo
	// Set node as Raft bootstrap cluster initiator - only a single node can do that
	// Default: false
	IsClusterInitiator bool
	// Logger.
	// Default: DefaultConsensusLogger
	Logger Logger
	// any custom validators. Note that validators don't affect Raft state
	Validators []ConsensusValidatorFunc
	// LibP2P endpoint for Raft nodes to be able to transfer incoming state change requests to the cluster leader
	LeaderProtocolID protocol.ID
	// Custom transport: by default, it's LibP2P TCP streams
	TransportFunc NewTransportFunc
}

type ConsensusService struct {
	ctx context.Context

	node  NodeTransporter
	cache votersCacher

	raftID         ServerID
	raft           *raft.Raft
	fsm            FSMachiner
	logStore       raft.LogStore
	stableStore    raft.StableStore
	snapshotStore  raft.SnapshotStore
	transportFunc  NewTransportFunc
	bootstrapNodes []peer.AddrInfo

	isClusterInitiator bool
	leaderEndpoint     protocol.ID
	codec              EncodeDecoder

	syncMx *sync.RWMutex

	l        Logger
	stopChan chan struct{}
	config   *RaftConfig
}

func NewLibP2pRaft(
	ctx context.Context,
	conf *Config,
) (_ *ConsensusService, err error) {
	var (
		stableStore   raft.StableStore
		snapshotStore raft.SnapshotStore
		logStore      raft.LogStore
	)
	if conf.Logger == nil {
		conf.Logger = DefaultConsensusLogger()
	}
	if conf.LogStore == nil {
		logStore = raft.NewInmemStore()
	}
	if conf.StableStore == nil {
		snapshotStore = raft.NewInmemSnapshotStore()
		stableStore = raft.NewInmemStore()
	} else {
		stableStore = conf.StableStore
		snapshotStore, err = raft.NewFileSnapshotStoreWithLogger(
			conf.StableStore.SnapshotsFilesPath(), 5, conf.Logger,
		)
		if err != nil {
			return nil, fmt.Errorf("consensus: failed to create snapshot store: %v", err)
		}
	}
	if conf.Codec == nil {
		conf.Codec = &DefaultCodec{}
	}
	if conf.FSM == nil {
		conf.FSM = NewFSM(conf.Codec, conf.Validators...)
	}
	if conf.HeartbeatTimeout == 0 {
		conf.HeartbeatTimeout = 5 * time.Second
	}
	if conf.CommitTimeout == 0 {
		conf.CommitTimeout = time.Second
	}
	if conf.SnapshotInterval == 0 {
		conf.SnapshotInterval = 20 * time.Minute
	}
	if conf.TrailingLogs == 0 {
		conf.TrailingLogs = 256
	}
	if conf.MaxAppendEntries == 0 {
		conf.MaxAppendEntries = 128
	}
	if conf.SnapshotThreshold == 0 {
		conf.SnapshotThreshold = 8192
	}
	if conf.TransportFunc == nil {
		conf.TransportFunc = NewConsensusTransport
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.HeartbeatTimeout = conf.HeartbeatTimeout
	raftConfig.ElectionTimeout = raftConfig.HeartbeatTimeout
	raftConfig.LeaderLeaseTimeout = raftConfig.HeartbeatTimeout
	raftConfig.CommitTimeout = conf.CommitTimeout
	raftConfig.MaxAppendEntries = int(conf.MaxAppendEntries)
	raftConfig.TrailingLogs = conf.TrailingLogs
	raftConfig.Logger = conf.Logger
	raftConfig.LocalID = ServerID("none")
	raftConfig.NoLegacyTelemetry = true
	raftConfig.SnapshotThreshold = conf.SnapshotThreshold
	raftConfig.SnapshotInterval = conf.SnapshotInterval
	raftConfig.NoSnapshotRestoreOnStart = true
	if err := raft.ValidateConfig(raftConfig); err != nil {
		return nil, err
	}

	return &ConsensusService{
		ctx:                ctx,
		logStore:           logStore,
		stableStore:        stableStore,
		snapshotStore:      snapshotStore,
		fsm:                conf.FSM,
		cache:              newVotersCache(),
		syncMx:             new(sync.RWMutex),
		l:                  conf.Logger,
		stopChan:           make(chan struct{}),
		config:             raftConfig,
		transportFunc:      conf.TransportFunc,
		bootstrapNodes:     conf.BootstrapNodes,
		isClusterInitiator: conf.IsClusterInitiator,
		leaderEndpoint:     conf.LeaderProtocolID,
		codec:              conf.Codec,
	}, nil
}

func (c *ConsensusService) Start(node NodeTransporter) (err error) {
	if c == nil {
		return errors.New("consensus: nil consensus service")
	}
	if node == nil {
		return errors.New("consensus: libp2p node not initialized")
	}

	c.syncMx.Lock()
	defer c.syncMx.Unlock()

	c.raftID = ServerID(node.ID().String())
	c.config.LocalID = c.raftID

	hasState, err := raft.HasExistingState(c.logStore, c.stableStore, c.snapshotStore)
	if err != nil {
		return fmt.Errorf("consensus: failed to check existing state: %v", err)
	}

	// the cluster doesn't already exist, and raft node has a status of an initiator
	if !hasState && c.isClusterInitiator {
		c.l.Info("consensus: setting up new cluster...")
		if err := c.bootstrap(c.config.LocalID); err != nil {
			return fmt.Errorf("consensus: setting up new cluster failed: %w", err)
		}
	}

	transport, err := c.transportFunc(node, c.l)
	if err != nil {
		return fmt.Errorf("consensus: setting up new transport failed: %w", err)
	}
	c.l.Info("consensus: transport configured with local address:", transport.LocalAddr())

	c.raft, err = raft.NewRaft(
		c.config,
		c.fsm,
		c.logStore,
		c.stableStore,
		c.snapshotStore,
		transport,
	)
	if err != nil {
		return fmt.Errorf("consensus: failed to create node: %w", err)
	}

	// wait cluster configuration to be updated
	if err := c.waitClusterReady(); err != nil {
		return err
	}

	// wait all nodes in cluster to be synced: leader elected and voters added
	if err = c.sync(); err != nil {
		return err
	}

	c.l.Info("consensus: ready node with last index:", c.raftID, c.raft.LastIndex())
	c.node = node
	go c.runLeadershipExpiration()

	return nil
}

// bootstrap method set up new cluster
func (c *ConsensusService) bootstrap(id ServerID) error {
	raftConf := raft.Configuration{}
	raftConf.Servers = append(raftConf.Servers, raft.Server{
		Suffrage: raft.Voter,
		ID:       id,
		Address:  ServerAddress(id),
	})
	for _, info := range c.bootstrapNodes {
		if string(id) == info.ID.String() {
			continue
		}
		raftConf.Servers = append(raftConf.Servers, raft.Server{
			Suffrage: raft.Voter,
			ID:       ServerID(info.ID.String()),
			Address:  ServerAddress(info.ID.String()),
		})
	}

	if err := c.stableStore.SetUint64([]byte("CurrentTerm"), 1); err != nil {
		return fmt.Errorf("consensus: failed to save current term: %v", err)
	}
	if err := c.logStore.StoreLog(&raft.Log{
		Type: raft.LogConfiguration, Index: 1, Term: 1,
		Data: raft.EncodeConfiguration(raftConf),
	}); err != nil {
		return fmt.Errorf("consensus: failed to store bootstrap log: %v", err)
	}

	return c.logStore.GetLog(1, &raft.Log{})
}

func (c *ConsensusService) waitClusterReady() error {
	clusterReadyChan := make(chan raft.ConfigurationFuture, 1)

	timeoutTimer := time.NewTimer(time.Second * 10)
	defer timeoutTimer.Stop()

	go func(crChan chan raft.ConfigurationFuture) {
		crChan <- c.raft.GetConfiguration()
	}(clusterReadyChan)

	select {
	case wait := <-clusterReadyChan:
		if wait.Error() != nil {
			return fmt.Errorf("consensus: config fetch error: %w", wait.Error())
		}
		c.l.Info("consensus: cluster is ready: servers list %s", wait.Configuration().Servers)
		break
	case <-timeoutTimer.C:
		return errors.New("consensus: getting configuration timeout — possibly broken cluster")
	}
	return nil
}

type consensusSync struct {
	ctx    context.Context
	raft   *raft.Raft
	raftID ServerID
	l      Logger
}

// sync is waiting own Raft node and other nodes to be up and running, having Leader or Followers status,
// recognizing each other, syncing state and sharing snapshots.
// You won't be able to run the cluster without that sync.
func (c *ConsensusService) sync() error {
	if c.raftID == "" {
		return errors.New("consensus: node id is not initialized")
	}
	if c.ctx.Err() != nil {
		return c.ctx.Err()
	}

	c.l.Info("consensus: waiting for sync...")

	leaderCtx, leaderCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer leaderCancel()

	cs := consensusSync{
		ctx:    c.ctx,
		raft:   c.raft,
		raftID: c.raftID,
		l:      c.l,
	}

	// find whose leader is the cluster
	if err := cs.waitForLeader(leaderCtx); err != nil {
		c.l.Warn("consensus: failed to wait for leadership sync: %v", err)
		return err
	}

	c.l.Info("consensus: waiting until we are promoted to a voter...")
	voterCtx, voterCancel := context.WithTimeout(context.Background(), time.Second*30)
	defer voterCancel()

	// sync own status. Am I a voter?
	if err := cs.waitForVoter(voterCtx); err != nil {
		c.l.Warn("consensus: waiting to become a voter:", err)
		return fmt.Errorf("consensus: waiting to become a voter: %w", err)
	}
	c.l.Info("consensus: node received voter status")

	updatesCtx, updatesCancel := context.WithTimeout(context.Background(), time.Minute)
	defer updatesCancel()

	// sync state
	if err := cs.waitForUpdates(updatesCtx); err != nil {
		c.l.Error("consensus: waiting for consensus updates:", err)
	}

	c.l.Info("consensus: sync complete")
	return nil
}

func (c *consensusSync) waitForLeader(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		select {
		case <-ticker.C:
			addr, leaderID := c.raft.LeaderWithID()
			if addr == "" {
				continue
			}
			if c.raftID == leaderID {
				c.l.Info("consensus: node is a leader!")
				return nil
			}
			c.l.Info("consensus: current leader: %s", leaderID)
			return nil

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (c *consensusSync) waitForVoter(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	id := c.raftID
	for {
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			wait := c.raft.GetConfiguration()
			if err := wait.Error(); err != nil {
				return err
			}

			if isVoter(id, wait.Configuration()) {
				return nil
			}
		}
	}
}

func (c *consensusSync) waitForUpdates(ctx context.Context) error {
	c.l.Debug("consensus: node state is catching up to the latest known version. Please wait...")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		if c.ctx.Err() != nil {
			return c.ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			lastAppliedIndex := c.raft.AppliedIndex()
			lastIndex := c.raft.LastIndex()

			c.l.Info("consensus: current node index: %d/%d", lastAppliedIndex, lastIndex)

			if lastAppliedIndex == lastIndex {
				return nil
			}
			if lastAppliedIndex > lastIndex {
				return errors.New("consensus: last applied index is greater than current index")
			}
		}
	}
}

func isVoter(srvID ServerID, cfg raft.Configuration) bool {
	for _, server := range cfg.Servers {
		if server.ID == srvID && (server.Suffrage == raft.Voter || server.Suffrage == raft.Nonvoter) {
			return true
		}
	}
	return false
}

func (c *ConsensusService) AddVoter(id ServerID) {
	c.waitSync()

	if c.raft == nil {
		return
	}
	if id == "" {
		return
	}

	peerId, err := peer.Decode(string(id))
	if err != nil {
		c.l.Error("consensus: add voter: invalid peer ID", id, err)
		return
	}
	addrInfo := c.node.Peerstore().PeerInfo(peerId)
	// let's check if libp2p node of this voter is accessible
	if err := c.node.Connect(c.ctx, addrInfo); err != nil {
		c.l.Error("consensus: add voter: failed to connect to peer:", id, err)
		return
	}

	if _, leaderId := c.raft.LeaderWithID(); c.raftID != leaderId {
		// only leader can add voters
		return
	}

	c.l.Debug("consensus: add voter: leader ID is", id)

	addr := ServerAddress(id)

	// check if voter already exists, check cluster capacity
	if _, err := c.cache.getVoter(id); errors.Is(err, errVoterNotFound) && c.cache.cap() >= MaxVotersNum {
		c.l.Error("consensus: add voter: failed to add voted: max capacity reached")
		c.raft.AddNonvoter(id, addr, 0, 30*time.Second)
		return
	}

	// add voter to stable store
	wait := c.raft.AddVoter(id, addr, 0, 30*time.Second)
	if wait.Error() != nil {
		c.l.Error("consensus: add voter: failed to add voted: ", wait.Error())
		return
	}
	c.l.Debug("consensus: new voter added:", id)

	if _, err := c.cache.getVoter(id); errors.Is(err, errVoterNotFound) {
		c.l.Info("consensus: new voter added: ", id)
	}

	// add voter to cache
	c.cache.addVoter(id, raft.Server{ // this cache only prevents voter removal from flapping
		Suffrage: raft.Voter,
		ID:       id,
		Address:  addr,
	})
	return
}

// RemoveVoter manually removes a voter from a cluster
func (c *ConsensusService) RemoveVoter(id ServerID) {
	if c.raft == nil {
		return
	}
	if id == "" {
		return
	}

	c.waitSync()

	if _, leaderId := c.raft.LeaderWithID(); c.raftID != leaderId {
		// not a leader - not allowed to remove voter
		return
	}

	err := c.cache.removeVoter(id)
	if errors.Is(err, errTooSoonToRemoveVoter) { // voter was fresh new - let's wait a bit and not remove it
		c.l.Info("consensus: removing voter is too soon, abort: ", id)
		return
	}
	if errors.Is(err, errVoterNotFound) { // was already demoted
		c.raft.RemoveServer(id, 0, 30*time.Second) // remove completely
		return
	}
	c.l.Info("consensus: removing voter:", id)

	// remove voter from stable store - this is final
	wait := c.raft.DemoteVoter(id, 0, 30*time.Second)
	if err := wait.Error(); err != nil {
		c.l.Error("consensus: failed to remove node:", wait.Error())
		return
	}
}

// Stats gives Raft node metrics
func (c *ConsensusService) Stats() map[string]string {
	return c.raft.Stats()
}

// ID retrieves current Raft node identificator
func (c *ConsensusService) ID() ServerID {
	if c.raft == nil {
		return "none"
	}
	return c.raftID
}

// LeaderID retrieves current cluster leader
func (c *ConsensusService) LeaderID() ServerID {
	if c.raft == nil {
		return "failed"
	}
	_, leaderId := c.raft.LeaderWithID()
	return leaderId
}

// AskValidation allows each node to request data validation from a Raft cluster leader
func (c *ConsensusService) AskValidation(key, value string) ([]byte, error) {
	c.l.Info("consensus: asking for validation ", key)
	return c.validate(KVState{key: value})
}

func (c *ConsensusService) validate(newState KVState) ([]byte, error) {
	if c.leaderEndpoint == "" {
		return nil, errors.New("consensus: no validation libp2p protocol provided")
	}
	leaderId := c.LeaderID()
	if leaderId == "" {
		return nil, errors.New("consensus: no leader found")
	}

	if leaderId == c.raftID {
		_, err := c.CommitState(newState)
		if errors.Is(err, ErrNoRaftCluster) {
			return nil, nil
		}
		return nil, err
	}

	leaderPeerId, err := peer.IDFromBytes([]byte(leaderId))
	if err != nil {
		return nil, err
	}

	resp, err := stream(c.ctx, c.node, leaderPeerId, c.leaderEndpoint, newState)
	if err != nil {
		return nil, fmt.Errorf("consensus: leader verify stream: %w", err)
	}
	if len(resp) == 0 {
		return nil, errors.New("consensus: node leader verify stream returned empty response")
	}

	return resp, nil
}

// CommitState updates Raft cluster state - only leader can do that
func (c *ConsensusService) CommitState(newState KVState) (_ *KVState, err error) {
	c.waitSync()

	if c.raft == nil {
		return nil, errors.New("consensus: commit: nil node")
	}

	wait := c.raft.GetConfiguration()
	if wait.Error() != nil {
		return nil, fmt.Errorf("consensus: commit: failed to get raft configuration: %w", wait.Error())
	}
	if len(wait.Configuration().Servers) <= 1 {
		return nil, ErrNoRaftCluster
	}

	if _, leaderId := c.raft.LeaderWithID(); c.raftID != leaderId {
		return nil, fmt.Errorf("consensus: commit: not a leader: %s", leaderId)
	}

	var buf bytes.Buffer
	if err := c.codec.Encode(newState, &buf); err != nil {
		return nil, err
	}

	result := c.raft.Apply(buf.Bytes(), time.Second*5)
	if result.Error() != nil {
		return nil, result.Error()
	}
	returnedState := result.Response()

	if kvState, ok := returnedState.(*KVState); ok {
		return kvState, nil
	}

	if err, ok := returnedState.(error); ok {
		return nil, err
	}

	return nil, fmt.Errorf("consensus: commit: failed: %v", returnedState)
}

// CurrentState retrieves last consensus state
func (c *ConsensusService) CurrentState() (*KVState, error) {
	c.waitSync()

	if c.raft == nil {
		return nil, errors.New("consensus: nil node")
	}

	currentState := c.fsm.GetCurrentState()
	return currentState, nil
}

// just semaphore
func (c *ConsensusService) waitSync() {
	c.syncMx.RLock()
	c.syncMx.RUnlock()
}

// just a dummy structure to prevent failed subscription panic
type dummySubscription struct {
	ch chan interface{}
}

func newDummySubscription() *dummySubscription {
	return &dummySubscription{make(chan interface{})}
}

func (d *dummySubscription) Close() error {
	return nil
}

func (d *dummySubscription) Out() <-chan interface{} {
	return d.ch
}

func (d *dummySubscription) Name() string {
	return "dummy"
}

// Raft cluster safety measure: libp2p node behind NAT if it's leader could cause whole cluster
// to be inaccessible.
// Constant leader changing to prevent single node overburdened
func (c *ConsensusService) runLeadershipExpiration() {
	c.waitSync()

	time.Sleep(time.Minute * 5)

	expirationTicker := time.NewTicker(time.Hour)
	defer expirationTicker.Stop()

	sub, err := c.node.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged))
	if err != nil {
		c.l.Warn("consensus: failed to subscribe to eventbus: %v", err)
	}
	if sub == nil {
		sub = newDummySubscription()
	}

	var (
		leaderID ServerID
		addr     ServerAddress
	)
	for {
		if c.ctx.Err() != nil {
			return
		}
		select {
		case <-c.stopChan:
			return
		case <-c.ctx.Done():
			return
		default:
		}
		if c.raft == nil {
			return
		}
		addr, leaderID = c.raft.LeaderWithID()
		if addr == "" {
			continue
		}
		if c.raftID != leaderID {
			continue
		}

		select {
		case <-expirationTicker.C:
			c.dropLeadership("leadership expiration") // constantly rotate leader
		case ev := <-sub.Out():
			r := ev.(event.EvtLocalReachabilityChanged).Reachability // it's int32 under the hood
			if r == network.ReachabilityPrivate {                    // unreachable private node could potentially block all consensus
				c.dropLeadership("private reachability")
			}
		}
	}
}

func (c *ConsensusService) dropLeadership(reason string) {
	if c == nil || c.raft == nil {
		return
	}

	peers := c.node.Network().Peers() // list of peers CONNECTED
	randomPeerID := peers[rand.Intn(len(peers))]

	c.l.Info("consensus: dropping leadership, transferring to %s, reason: %s", randomPeerID, reason)

	wait := c.raft.LeadershipTransferToServer(
		ServerID(randomPeerID.String()), ServerAddress(randomPeerID.String()),
	)
	if wait.Error() == nil {
		return
	}
	c.l.Error(
		"consensus: failed to send leader ship transfer to server %s: %v",
		randomPeerID.String(), wait.Error(),
	)
}

func (c *ConsensusService) Shutdown() {
	if c == nil || c.raft == nil {
		return
	}
	defer func() { recover() }()

	close(c.stopChan)

	wait := c.raft.Shutdown()
	if wait != nil && wait.Error() != nil {
		c.l.Info("consensus: failed to shutdown node: %v", wait.Error())
	}
	c.raft = nil
	c.l.Info("consensus: node shut down")

}
