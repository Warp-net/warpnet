package raft

import (
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"time"
)

type (
	AddrInfo   = peer.AddrInfo
	ProtocolID = protocol.ID
)

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

type Option func(*Config)

func applyOptions(opts []Option) *Config {
	cfg := &Config{}
	for _, opt := range opts {
		opt(cfg)
	}
	setDefaults(cfg)
	return cfg
}

func setDefaults(cfg *Config) {
	if cfg.Logger == nil {
		cfg.Logger = DefaultConsensusLogger()
	}
	if cfg.Codec == nil {
		cfg.Codec = &DefaultCodec{}
	}
	if cfg.FSM == nil {
		cfg.FSM = NewFSM(cfg.Codec, cfg.Validators...)
	}
	if cfg.HeartbeatTimeout == 0 {
		cfg.HeartbeatTimeout = 5 * time.Second
	}
	if cfg.CommitTimeout == 0 {
		cfg.CommitTimeout = time.Second
	}
	if cfg.SnapshotInterval == 0 {
		cfg.SnapshotInterval = 20 * time.Minute
	}
	if cfg.TrailingLogs == 0 {
		cfg.TrailingLogs = 256
	}
	if cfg.MaxAppendEntries == 0 {
		cfg.MaxAppendEntries = 128
	}
	if cfg.SnapshotThreshold == 0 {
		cfg.SnapshotThreshold = 8192
	}
	if cfg.TransportFunc == nil {
		cfg.TransportFunc = NewConsensusTransport
	}
}

func WithLogger(logger Logger) Option           { return func(c *Config) { c.Logger = logger } }
func WithFSM(fsm FSMachiner) Option             { return func(c *Config) { c.FSM = fsm } }
func WithCodec(codec EncodeDecoder) Option      { return func(c *Config) { c.Codec = codec } }
func WithStableStore(store StableStorer) Option { return func(c *Config) { c.StableStore = store } }
func WithLogStore(store LogStore) Option        { return func(c *Config) { c.LogStore = store } }
func WithBootstrapNodes(nodes []AddrInfo) Option {
	return func(c *Config) { c.BootstrapNodes = nodes }
}
func WithClusterInitiator(flag bool) Option             { return func(c *Config) { c.IsClusterInitiator = flag } }
func WithValidators(v ...ConsensusValidatorFunc) Option { return func(c *Config) { c.Validators = v } }
func WithLeaderProtocolID(pid ProtocolID) Option {
	return func(c *Config) { c.LeaderProtocolID = pid }
}
func WithTransportFunc(fn NewTransportFunc) Option { return func(c *Config) { c.TransportFunc = fn } }
func WithHeartbeatTimeout(d time.Duration) Option  { return func(c *Config) { c.HeartbeatTimeout = d } }
func WithCommitTimeout(d time.Duration) Option     { return func(c *Config) { c.CommitTimeout = d } }
func WithSnapshotInterval(d time.Duration) Option  { return func(c *Config) { c.SnapshotInterval = d } }
func WithMaxAppendEntries(v uint64) Option         { return func(c *Config) { c.MaxAppendEntries = v } }
func WithTrailingLogs(v uint64) Option             { return func(c *Config) { c.TrailingLogs = v } }
func WithSnapshotThreshold(v uint64) Option        { return func(c *Config) { c.SnapshotThreshold = v } }
