import Foundation

public struct WarpnetTweet: Codable, Equatable, Sendable {
  public var id: String
  public var createdAt: String?
  public var updatedAt: String?
  public var parentId: String?
  public var parentUserId: String?
  public var retweetedBy: String?
  public var rootId: String
  public var text: String
  public var userId: String
  public var username: String
  public var imageKeys: [String]?
  public var videoKey: String?
  public var network: String
  public var pinned: Bool
  public var quotedTweetId: String?
  public var quotedUserId: String?
  public var poll: WarpnetPoll?

  enum CodingKeys: String, CodingKey {
    case id
    case createdAt = "created_at"
    case updatedAt = "updated_at"
    case parentId = "parent_id"
    case parentUserId = "parent_user_id"
    case retweetedBy = "retweeted_by"
    case rootId = "root_id"
    case text
    case userId = "user_id"
    case username
    case imageKeys = "image_keys"
    case videoKey = "video_key"
    case network
    case pinned
    case quotedTweetId = "quoted_tweet_id"
    case quotedUserId = "quoted_user_id"
    case poll
  }

  public init(id: String,
              createdAt: String? = nil,
              updatedAt: String? = nil,
              parentId: String? = nil,
              parentUserId: String? = nil,
              retweetedBy: String? = nil,
              rootId: String = "",
              text: String,
              userId: String,
              username: String,
              imageKeys: [String]? = nil,
              videoKey: String? = nil,
              network: String = "",
              pinned: Bool = false,
              quotedTweetId: String? = nil,
              quotedUserId: String? = nil,
              poll: WarpnetPoll? = nil)
  {
    self.id = id
    self.createdAt = createdAt
    self.updatedAt = updatedAt
    self.parentId = parentId
    self.parentUserId = parentUserId
    self.retweetedBy = retweetedBy
    self.rootId = rootId
    self.text = text
    self.userId = userId
    self.username = username
    self.imageKeys = imageKeys
    self.videoKey = videoKey
    self.network = network
    self.pinned = pinned
    self.quotedTweetId = quotedTweetId
    self.quotedUserId = quotedUserId
    self.poll = poll
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    createdAt = try c.decodeIfPresent(String.self, forKey: .createdAt)
    updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
    parentId = try c.decodeIfPresent(String.self, forKey: .parentId)
    parentUserId = try c.decodeIfPresent(String.self, forKey: .parentUserId)
    retweetedBy = try c.decodeIfPresent(String.self, forKey: .retweetedBy)
    rootId = try c.value(.rootId, "")
    text = try c.value(.text, "")
    userId = try c.value(.userId, "")
    username = try c.value(.username, "")
    imageKeys = try c.decodeIfPresent([String].self, forKey: .imageKeys)
    videoKey = try c.decodeIfPresent(String.self, forKey: .videoKey)
    network = try c.value(.network, "")
    pinned = try c.value(.pinned, false)
    quotedTweetId = try c.decodeIfPresent(String.self, forKey: .quotedTweetId)
    quotedUserId = try c.decodeIfPresent(String.self, forKey: .quotedUserId)
    poll = try c.decodeIfPresent(WarpnetPoll.self, forKey: .poll)
  }
}

public struct WarpnetPoll: Codable, Equatable, Sendable {
  public var options: [String]
  public var expiresAt: String

  enum CodingKeys: String, CodingKey {
    case options
    case expiresAt = "expires_at"
  }

  public init(options: [String] = [], expiresAt: String = "") {
    self.options = options
    self.expiresAt = expiresAt
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    options = try c.value(.options, [])
    expiresAt = try c.value(.expiresAt, "")
  }
}

public struct WarpnetUser: Codable, Equatable, Sendable {
  public var id: String
  public var username: String
  public var bio: String
  public var birthdate: String
  public var avatarKey: String?
  public var backgroundImageKey: String
  public var createdAt: String
  public var updatedAt: String?
  public var followingsCount: Int
  public var followersCount: Int
  public var tweetsCount: Int
  public var isOffline: Bool
  public var nodeId: String
  public var network: String
  public var website: String?
  public var locked: Bool

  enum CodingKeys: String, CodingKey {
    case id
    case username
    case bio
    case birthdate
    case avatarKey = "avatar_key"
    case backgroundImageKey = "background_image_key"
    case createdAt = "created_at"
    case updatedAt = "updated_at"
    case followingsCount = "followings_count"
    case followersCount = "followers_count"
    case tweetsCount = "tweets_count"
    case isOffline
    case nodeId = "node_id"
    case network
    case website
    case locked
  }

  public init(id: String,
              username: String,
              bio: String = "",
              birthdate: String = "",
              avatarKey: String? = nil,
              backgroundImageKey: String = "",
              createdAt: String = "",
              updatedAt: String? = nil,
              followingsCount: Int = 0,
              followersCount: Int = 0,
              tweetsCount: Int = 0,
              isOffline: Bool = false,
              nodeId: String = "",
              network: String = "",
              website: String? = nil,
              locked: Bool = false)
  {
    self.id = id
    self.username = username
    self.bio = bio
    self.birthdate = birthdate
    self.avatarKey = avatarKey
    self.backgroundImageKey = backgroundImageKey
    self.createdAt = createdAt
    self.updatedAt = updatedAt
    self.followingsCount = followingsCount
    self.followersCount = followersCount
    self.tweetsCount = tweetsCount
    self.isOffline = isOffline
    self.nodeId = nodeId
    self.network = network
    self.website = website
    self.locked = locked
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    username = try c.value(.username, "")
    bio = try c.value(.bio, "")
    birthdate = try c.value(.birthdate, "")
    avatarKey = try c.decodeIfPresent(String.self, forKey: .avatarKey)
    backgroundImageKey = try c.value(.backgroundImageKey, "")
    createdAt = try c.value(.createdAt, "")
    updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
    followingsCount = try c.value(.followingsCount, 0)
    followersCount = try c.value(.followersCount, 0)
    tweetsCount = try c.value(.tweetsCount, 0)
    isOffline = try c.value(.isOffline, false)
    nodeId = try c.value(.nodeId, "")
    network = try c.value(.network, "")
    website = try c.decodeIfPresent(String.self, forKey: .website)
    locked = try c.value(.locked, false)
  }
}

public struct WarpnetNotification: Codable, Equatable, Sendable {
  public var id: String
  public var createdAt: String
  public var type: String
  public var text: String
  public var userId: String
  public var actorId: String?
  public var tweetId: String?
  public var isRead: Bool

  enum CodingKeys: String, CodingKey {
    case id
    case createdAt = "created_at"
    case type
    case text
    case userId = "user_id"
    case actorId = "actor_id"
    case tweetId = "tweet_id"
    case isRead = "is_read"
  }

  public init(id: String,
              createdAt: String,
              type: String,
              text: String = "",
              userId: String = "",
              actorId: String? = nil,
              tweetId: String? = nil,
              isRead: Bool = false)
  {
    self.id = id
    self.createdAt = createdAt
    self.type = type
    self.text = text
    self.userId = userId
    self.actorId = actorId
    self.tweetId = tweetId
    self.isRead = isRead
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    createdAt = try c.value(.createdAt, "")
    type = try c.value(.type, "")
    text = try c.value(.text, "")
    userId = try c.value(.userId, "")
    actorId = try c.decodeIfPresent(String.self, forKey: .actorId)
    tweetId = try c.decodeIfPresent(String.self, forKey: .tweetId)
    isRead = try c.value(.isRead, false)
  }
}

public struct GatewaySettings: Decodable, Equatable, Sendable {
  public var nodeId: String

  enum CodingKeys: String, CodingKey {
    case nodeId = "node_id"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    nodeId = try c.value(.nodeId, "")
  }
}

public struct TweetsResponse: Decodable, Equatable, Sendable {
  public var tweets: [WarpnetTweet]
  public var cursor: String

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    tweets = try c.value(.tweets, [])
    cursor = try c.value(.cursor, "")
  }

  enum CodingKeys: String, CodingKey {
    case tweets
    case cursor
  }
}

public struct UsersResponse: Decodable, Equatable, Sendable {
  public var users: [WarpnetUser]
  public var cursor: String

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    users = try c.value(.users, [])
    cursor = try c.value(.cursor, "")
  }

  enum CodingKeys: String, CodingKey {
    case users
    case cursor
  }
}

public struct ReactionsCountResponse: Decodable, Equatable, Sendable {
  public var reactionsCount: Int
  public var reactions: [String: Int]

  enum CodingKeys: String, CodingKey {
    case reactionsCount = "count"
    case reactions
  }

  public init(reactionsCount: Int = 0, reactions: [String: Int] = [:]) {
    self.reactionsCount = reactionsCount
    self.reactions = reactions
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    reactionsCount = try c.value(.reactionsCount, 0)
    reactions = try c.value(.reactions, [:])
  }
}

public struct ViewsCountResponse: Decodable, Equatable, Sendable {
  public var count: Int

  enum CodingKeys: String, CodingKey {
    case count
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    count = try c.value(.count, 0)
  }
}

public struct GetNotificationsResponse: Decodable, Equatable, Sendable {
  public var notifications: [WarpnetNotification]
  public var cursor: String
  public var unreadCount: Int

  enum CodingKeys: String, CodingKey {
    case notifications
    case cursor
    case unreadCount = "unread_count"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    notifications = try c.value(.notifications, [])
    cursor = try c.value(.cursor, "")
    unreadCount = try c.value(.unreadCount, 0)
  }
}

public struct FollowersResponse: Decodable, Equatable, Sendable {
  public var followers: [String]
  public var cursor: String
  public var followingId: String

  enum CodingKeys: String, CodingKey {
    case followers
    case cursor
    case followingId = "following_id"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    followers = try c.value(.followers, [])
    cursor = try c.value(.cursor, "")
    followingId = try c.value(.followingId, "")
  }
}

public struct FollowingsResponse: Decodable, Equatable, Sendable {
  public var followings: [String]
  public var cursor: String
  public var followerId: String

  enum CodingKeys: String, CodingKey {
    case followings
    case cursor
    case followerId = "follower_id"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    followings = try c.value(.followings, [])
    cursor = try c.value(.cursor, "")
    followerId = try c.value(.followerId, "")
  }
}

public struct IsFollowingResponse: Decodable, Equatable, Sendable {
  public var isFollowing: Bool

  enum CodingKeys: String, CodingKey {
    case isFollowing = "is_following"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    isFollowing = try c.value(.isFollowing, false)
  }
}

public struct IsFollowerResponse: Decodable, Equatable, Sendable {
  public var isFollower: Bool

  enum CodingKeys: String, CodingKey {
    case isFollower = "is_follower"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    isFollower = try c.value(.isFollower, false)
  }
}

public struct TweetStatsResponse: Decodable, Equatable, Sendable {
  public var tweetId: String
  public var retweetsCount: Int
  public var reactionsCount: Int
  public var repliesCount: Int
  public var viewsCount: Int
  public var reactions: [String: Int]
  public var myReaction: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case retweetsCount = "retweets_count"
    case reactionsCount = "reactions_count"
    case repliesCount = "replies_count"
    case viewsCount = "views_count"
    case reactions
    case myReaction = "my_reaction"
  }

  public init(tweetId: String = "",
              retweetsCount: Int = 0,
              reactionsCount: Int = 0,
              repliesCount: Int = 0,
              viewsCount: Int = 0,
              reactions: [String: Int] = [:],
              myReaction: String = "")
  {
    self.tweetId = tweetId
    self.retweetsCount = retweetsCount
    self.reactionsCount = reactionsCount
    self.repliesCount = repliesCount
    self.viewsCount = viewsCount
    self.reactions = reactions
    self.myReaction = myReaction
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    tweetId = try c.value(.tweetId, "")
    retweetsCount = try c.value(.retweetsCount, 0)
    reactionsCount = try c.value(.reactionsCount, 0)
    repliesCount = try c.value(.repliesCount, 0)
    viewsCount = try c.value(.viewsCount, 0)
    reactions = try c.value(.reactions, [:])
    myReaction = try c.value(.myReaction, "")
  }
}

public struct BookmarkItem: Decodable, Equatable, Sendable {
  public var userId: String
  public var tweetId: String
  public var ownerUserId: String
  public var createdAt: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case tweetId = "tweet_id"
    case ownerUserId = "owner_user_id"
    case createdAt = "created_at"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    userId = try c.value(.userId, "")
    tweetId = try c.value(.tweetId, "")
    ownerUserId = try c.value(.ownerUserId, "")
    createdAt = try c.value(.createdAt, "")
  }
}

public struct GetBookmarksResponse: Decodable, Equatable, Sendable {
  public var items: [BookmarkItem]
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case items
    case cursor
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    items = try c.value(.items, [])
    cursor = try c.value(.cursor, "")
  }
}

public struct GetBlocksResponse: Decodable, Equatable, Sendable {
  public var ids: [String]
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case ids
    case cursor
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    ids = try c.value(.ids, [])
    cursor = try c.value(.cursor, "")
  }
}

public struct PollResultsResponse: Decodable, Equatable, Sendable {
  public var tweetId: String
  public var votes: [Int]
  public var totalVotes: Int
  public var votedOption: Int?

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case votes
    case totalVotes = "total_votes"
    case votedOption = "voted_option"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    tweetId = try c.value(.tweetId, "")
    votes = try c.value(.votes, [])
    totalVotes = try c.value(.totalVotes, 0)
    votedOption = try c.decodeIfPresent(Int.self, forKey: .votedOption)
  }
}

public struct GetFollowRequestsResponse: Decodable, Equatable, Sendable {
  public var followerIds: [String]
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case followerIds = "follower_ids"
    case cursor
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    followerIds = try c.value(.followerIds, [])
    cursor = try c.value(.cursor, "")
  }
}

public struct WarpnetFilterKeyword: Codable, Equatable, Sendable {
  public var id: String
  public var keyword: String
  public var wholeWord: Bool

  enum CodingKeys: String, CodingKey {
    case id
    case keyword
    case wholeWord = "whole_word"
  }

  public init(id: String = "", keyword: String = "", wholeWord: Bool = false) {
    self.id = id
    self.keyword = keyword
    self.wholeWord = wholeWord
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    keyword = try c.value(.keyword, "")
    wholeWord = try c.value(.wholeWord, false)
  }
}

public struct WarpnetFilter: Codable, Equatable, Sendable {
  public var id: String
  public var userId: String
  public var title: String
  public var context: [String]
  public var action: String
  public var expiresAt: String?
  public var keywords: [WarpnetFilterKeyword]

  enum CodingKeys: String, CodingKey {
    case id
    case userId = "user_id"
    case title
    case context
    case action
    case expiresAt = "expires_at"
    case keywords
  }

  public init(id: String = "",
              userId: String = "",
              title: String = "",
              context: [String] = [],
              action: String = "warn",
              expiresAt: String? = nil,
              keywords: [WarpnetFilterKeyword] = [])
  {
    self.id = id
    self.userId = userId
    self.title = title
    self.context = context
    self.action = action
    self.expiresAt = expiresAt
    self.keywords = keywords
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    userId = try c.value(.userId, "")
    title = try c.value(.title, "")
    context = try c.value(.context, [])
    action = try c.value(.action, "warn")
    expiresAt = try c.decodeIfPresent(String.self, forKey: .expiresAt)
    keywords = try c.value(.keywords, [])
  }
}

public struct GetFiltersResponse: Decodable, Equatable, Sendable {
  public var filters: [WarpnetFilter]
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case filters
    case cursor
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    filters = try c.value(.filters, [])
    cursor = try c.value(.cursor, "")
  }
}

public struct WarpnetChat: Decodable, Equatable, Sendable {
  public var id: String
  public var ownerId: String
  public var otherUserId: String
  public var lastMessage: String
  public var createdAt: String
  public var updatedAt: String

  enum CodingKeys: String, CodingKey {
    case id
    case ownerId = "owner_id"
    case otherUserId = "other_user_id"
    case lastMessage = "last_message"
    case createdAt = "created_at"
    case updatedAt = "updated_at"
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    ownerId = try c.value(.ownerId, "")
    otherUserId = try c.value(.otherUserId, "")
    lastMessage = try c.value(.lastMessage, "")
    createdAt = try c.value(.createdAt, "")
    updatedAt = try c.value(.updatedAt, "")
  }
}

public struct GetChatsResponse: Decodable, Equatable, Sendable {
  public var chats: [WarpnetChat]
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case chats
    case cursor
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    chats = try c.value(.chats, [])
    cursor = try c.value(.cursor, "")
  }
}

public struct GetImageResponse: Decodable, Equatable, Sendable {
  public var file: String

  enum CodingKeys: String, CodingKey {
    case file
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    file = try c.value(.file, "")
  }
}

public struct UploadImageResponse: Decodable, Equatable, Sendable {
  public var key1: String
  public var key2: String
  public var key3: String
  public var key4: String

  enum CodingKeys: String, CodingKey {
    case key1
    case key2
    case key3
    case key4
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    key1 = try c.value(.key1, "")
    key2 = try c.value(.key2, "")
    key3 = try c.value(.key3, "")
    key4 = try c.value(.key4, "")
  }

  public var keys: [String] {
    [key1, key2, key3, key4].filter { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
  }
}

public struct UploadVideoResponse: Decodable, Equatable, Sendable {
  public var key: String

  enum CodingKeys: String, CodingKey {
    case key
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    key = try c.value(.key, "")
  }
}

public struct GetVideoResponse: Decodable, Equatable, Sendable {
  public var file: String
  public var size: Int
  public var deferred: Bool

  enum CodingKeys: String, CodingKey {
    case file
    case size
    case deferred
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    file = try c.value(.file, "")
    size = try c.value(.size, 0)
    deferred = try c.value(.deferred, false)
  }
}

public struct WarpnetMessage: Decodable, Equatable, Sendable {
  public var id: String
  public var chatId: String
  public var senderId: String
  public var receiverId: String
  public var text: String
  public var createdAt: String
  public var status: String

  enum CodingKeys: String, CodingKey {
    case id
    case chatId = "chat_id"
    case senderId = "sender_id"
    case receiverId = "receiver_id"
    case text
    case createdAt = "created_at"
    case status
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    id = try c.value(.id, "")
    chatId = try c.value(.chatId, "")
    senderId = try c.value(.senderId, "")
    receiverId = try c.value(.receiverId, "")
    text = try c.value(.text, "")
    createdAt = try c.value(.createdAt, "")
    status = try c.value(.status, "")
  }
}

public struct GetMessagesResponse: Decodable, Equatable, Sendable {
  public var chatId: String
  public var cursor: String
  public var messages: [WarpnetMessage]

  enum CodingKeys: String, CodingKey {
    case chatId = "chat_id"
    case cursor
    case messages
  }

  public init(from decoder: Decoder) throws {
    let c = try decoder.container(keyedBy: CodingKeys.self)
    chatId = try c.value(.chatId, "")
    cursor = try c.value(.cursor, "")
    messages = try c.value(.messages, [])
  }
}

extension KeyedDecodingContainer {
  func value<T: Decodable>(_ key: Key, _ fallback: T) throws -> T {
    try decodeIfPresent(T.self, forKey: key) ?? fallback
  }
}
