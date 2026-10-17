import Foundation

public struct GetUserEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var nodeId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case nodeId = "node_id"
  }

  public init(userId: String, nodeId: String = "") {
    self.userId = userId
    self.nodeId = nodeId
  }
}

public struct GetAllTweetsEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var rootId: String
  public var parentId: String
  public var rootUserId: String
  public var cursor: String
  public var limit: Int

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case rootId = "root_id"
    case parentId = "parent_id"
    case rootUserId = "root_user_id"
    case cursor
    case limit
  }

  public init(userId: String = "",
              rootId: String = "",
              parentId: String = "",
              rootUserId: String = "",
              cursor: String = "",
              limit: Int = 40)
  {
    self.userId = userId
    self.rootId = rootId
    self.parentId = parentId
    self.rootUserId = rootUserId
    self.cursor = cursor
    self.limit = limit
  }
}

public struct GetTweetEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var parentId: String
  public var rootId: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case parentId = "parent_id"
    case rootId = "root_id"
  }

  public init(tweetId: String, userId: String, parentId: String = "", rootId: String = "") {
    self.tweetId = tweetId
    self.userId = userId
    self.parentId = parentId
    self.rootId = rootId
  }
}

public typealias DeleteTweetEvent = GetTweetEvent

public struct NewFollowEvent: Encodable, Equatable, Sendable {
  public var followerId: String
  public var followeeId: String

  enum CodingKeys: String, CodingKey {
    case followerId = "follower_id"
    case followeeId = "following_id"
  }

  public init(followerId: String, followeeId: String) {
    self.followerId = followerId
    self.followeeId = followeeId
  }
}

public typealias NewUnfollowEvent = NewFollowEvent

public struct ReactionEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var ownerId: String
  public var emoji: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case ownerId = "owner_id"
    case emoji
  }

  public init(tweetId: String, userId: String, ownerId: String, emoji: String = "") {
    self.tweetId = tweetId
    self.userId = userId
    self.ownerId = ownerId
    self.emoji = emoji
  }
}

public struct ViewEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var viewerId: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case viewerId = "viewer_id"
  }

  public init(tweetId: String, userId: String, viewerId: String) {
    self.tweetId = tweetId
    self.userId = userId
    self.viewerId = viewerId
  }
}

public struct GetTweetStatsEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
  }

  public init(tweetId: String, userId: String) {
    self.tweetId = tweetId
    self.userId = userId
  }
}

public struct UnretweetEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var retweeterId: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case retweeterId = "retweeter_id"
  }

  public init(tweetId: String, retweeterId: String) {
    self.tweetId = tweetId
    self.retweeterId = retweeterId
  }
}

public struct GetNotificationsEvent: Encodable, Equatable, Sendable {
  public var cursor: String
  public var limit: Int

  public init(cursor: String = "", limit: Int = 40) {
    self.cursor = cursor
    self.limit = limit
  }
}

public struct GetNotificationEvent: Encodable, Equatable, Sendable {
  public var notificationId: String

  enum CodingKeys: String, CodingKey {
    case notificationId = "notification_id"
  }

  public init(notificationId: String) {
    self.notificationId = notificationId
  }
}

public typealias MarkNotificationReadEvent = GetNotificationEvent

public struct BookmarkEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var tweetId: String
  public var ownerUserId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case tweetId = "tweet_id"
    case ownerUserId = "owner_user_id"
  }

  public init(userId: String, tweetId: String, ownerUserId: String) {
    self.userId = userId
    self.tweetId = tweetId
    self.ownerUserId = ownerUserId
  }
}

public struct UnbookmarkEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var tweetId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case tweetId = "tweet_id"
  }

  public init(userId: String, tweetId: String) {
    self.userId = userId
    self.tweetId = tweetId
  }
}

public struct GetBookmarksEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var cursor: String
  public var limit: Int

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case cursor
    case limit
  }

  public init(userId: String, cursor: String = "", limit: Int = 40) {
    self.userId = userId
    self.cursor = cursor
    self.limit = limit
  }
}

public typealias GetBlocksEvent = GetBookmarksEvent
public typealias GetFollowRequestsEvent = GetBookmarksEvent
public typealias GetFiltersEvent = GetBookmarksEvent
public typealias GetFollowersEvent = GetBookmarksEvent
public typealias GetFollowingsEvent = GetBookmarksEvent
public typealias GetAllUsersEvent = GetBookmarksEvent
public typealias GetChatsEvent = GetBookmarksEvent

public struct PinTweetEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var tweetId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case tweetId = "tweet_id"
  }

  public init(userId: String, tweetId: String) {
    self.userId = userId
    self.tweetId = tweetId
  }
}

public struct BlockEvent: Encodable, Equatable, Sendable {
  public var blockerId: String
  public var blockeeId: String

  enum CodingKeys: String, CodingKey {
    case blockerId = "blocker_id"
    case blockeeId = "blockee_id"
  }

  public init(blockerId: String, blockeeId: String) {
    self.blockerId = blockerId
    self.blockeeId = blockeeId
  }
}

public struct MuteEvent: Encodable, Equatable, Sendable {
  public var muterId: String
  public var muteeId: String

  enum CodingKeys: String, CodingKey {
    case muterId = "muter_id"
    case muteeId = "mutee_id"
  }

  public init(muterId: String, muteeId: String) {
    self.muterId = muterId
    self.muteeId = muteeId
  }
}

public struct GetTweetReactorsEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var ownerUserId: String
  public var cursor: String
  public var limit: Int

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case ownerUserId = "owner_user_id"
    case cursor
    case limit
  }

  public init(tweetId: String, ownerUserId: String, cursor: String = "", limit: Int = 40) {
    self.tweetId = tweetId
    self.ownerUserId = ownerUserId
    self.cursor = cursor
    self.limit = limit
  }
}

public struct SubscribeUserEvent: Encodable, Equatable, Sendable {
  public var selfId: String
  public var targetId: String

  enum CodingKeys: String, CodingKey {
    case selfId = "self_id"
    case targetId = "target_id"
  }

  public init(selfId: String, targetId: String) {
    self.selfId = selfId
    self.targetId = targetId
  }
}

public struct PollVoteEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var ownerId: String
  public var option: Int
  public var optionsNum: Int

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case ownerId = "owner_id"
    case option
    case optionsNum = "options_num"
  }

  public init(tweetId: String, userId: String, ownerId: String, option: Int, optionsNum: Int) {
    self.tweetId = tweetId
    self.userId = userId
    self.ownerId = ownerId
    self.option = option
    self.optionsNum = optionsNum
  }
}

public struct GetPollEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var ownerId: String
  public var optionsNum: Int

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case ownerId = "owner_id"
    case optionsNum = "options_num"
  }

  public init(tweetId: String, userId: String, ownerId: String, optionsNum: Int) {
    self.tweetId = tweetId
    self.userId = userId
    self.ownerId = ownerId
    self.optionsNum = optionsNum
  }
}

public struct SearchUsersEvent: Encodable, Equatable, Sendable {
  public var query: String
  public var cursor: String
  public var limit: Int

  public init(query: String, cursor: String = "", limit: Int = 40) {
    self.query = query
    self.cursor = cursor
    self.limit = limit
  }
}

public struct EditTweetEvent: Encodable, Equatable, Sendable {
  public var tweetId: String
  public var userId: String
  public var text: String

  enum CodingKeys: String, CodingKey {
    case tweetId = "tweet_id"
    case userId = "user_id"
    case text
  }

  public init(tweetId: String, userId: String, text: String) {
    self.tweetId = tweetId
    self.userId = userId
    self.text = text
  }
}

public struct FollowRequestActionEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var followerId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case followerId = "follower_id"
  }

  public init(userId: String, followerId: String) {
    self.userId = userId
    self.followerId = followerId
  }
}

public struct GetFilterEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var filterId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case filterId = "filter_id"
  }

  public init(userId: String, filterId: String) {
    self.userId = userId
    self.filterId = filterId
  }
}

public typealias DeleteFilterEvent = GetFilterEvent

public struct AddFilterKeywordEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var filterId: String
  public var keyword: String
  public var wholeWord: Bool

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case filterId = "filter_id"
    case keyword
    case wholeWord = "whole_word"
  }

  public init(userId: String, filterId: String, keyword: String, wholeWord: Bool = false) {
    self.userId = userId
    self.filterId = filterId
    self.keyword = keyword
    self.wholeWord = wholeWord
  }
}

public struct UpdateFilterKeywordEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var keywordId: String
  public var keyword: String
  public var wholeWord: Bool

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case keywordId = "keyword_id"
    case keyword
    case wholeWord = "whole_word"
  }

  public init(userId: String, keywordId: String, keyword: String = "", wholeWord: Bool = false) {
    self.userId = userId
    self.keywordId = keywordId
    self.keyword = keyword
    self.wholeWord = wholeWord
  }
}

public struct DeleteFilterKeywordEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var keywordId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case keywordId = "keyword_id"
  }

  public init(userId: String, keywordId: String) {
    self.userId = userId
    self.keywordId = keywordId
  }
}

public struct GetIsFollowingEvent: Encodable, Equatable, Sendable {
  public var userId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
  }

  public init(userId: String) {
    self.userId = userId
  }
}

public struct NewChatEvent: Encodable, Equatable, Sendable {
  public var ownerId: String
  public var otherUserId: String

  enum CodingKeys: String, CodingKey {
    case ownerId = "owner_id"
    case otherUserId = "other_user_id"
  }

  public init(ownerId: String, otherUserId: String) {
    self.ownerId = ownerId
    self.otherUserId = otherUserId
  }
}

public struct GetChatEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var chatId: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case chatId = "chat_id"
  }

  public init(userId: String, chatId: String) {
    self.userId = userId
    self.chatId = chatId
  }
}

public typealias DeleteChatEvent = GetChatEvent

public struct GetImageEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var key: String

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case key
  }

  public init(userId: String, key: String) {
    self.userId = userId
    self.key = key
  }
}

public struct UploadImageEvent: Encodable, Equatable, Sendable {
  public var image1: String
  public var image2: String
  public var image3: String
  public var image4: String

  public init(image1: String = "", image2: String = "", image3: String = "", image4: String = "") {
    self.image1 = image1
    self.image2 = image2
    self.image3 = image3
    self.image4 = image4
  }
}

public struct UploadVideoEvent: Encodable, Equatable, Sendable {
  public var video: String

  public init(video: String) {
    self.video = video
  }
}

public struct GetVideoEvent: Encodable, Equatable, Sendable {
  public var userId: String
  public var key: String
  public var deferred: Bool

  enum CodingKeys: String, CodingKey {
    case userId = "user_id"
    case key
    case deferred
  }

  public init(userId: String, key: String, deferred: Bool = false) {
    self.userId = userId
    self.key = key
    self.deferred = deferred
  }
}

public struct GetMessagesEvent: Encodable, Equatable, Sendable {
  public var ownerId: String
  public var chatId: String
  public var limit: Int
  public var cursor: String

  enum CodingKeys: String, CodingKey {
    case ownerId = "owner_id"
    case chatId = "chat_id"
    case limit
    case cursor
  }

  public init(ownerId: String, chatId: String, limit: Int = 40, cursor: String = "") {
    self.ownerId = ownerId
    self.chatId = chatId
    self.limit = limit
    self.cursor = cursor
  }
}

public struct NewMessageEvent: Encodable, Equatable, Sendable {
  public var chatId: String
  public var senderId: String
  public var receiverId: String
  public var text: String

  enum CodingKeys: String, CodingKey {
    case chatId = "chat_id"
    case senderId = "sender_id"
    case receiverId = "receiver_id"
    case text
  }

  public init(chatId: String, senderId: String, receiverId: String, text: String) {
    self.chatId = chatId
    self.senderId = senderId
    self.receiverId = receiverId
    self.text = text
  }
}

public struct DeleteMessageEvent: Encodable, Equatable, Sendable {
  public var chatId: String
  public var id: String

  enum CodingKeys: String, CodingKey {
    case chatId = "chat_id"
    case id
  }

  public init(chatId: String, id: String) {
    self.chatId = chatId
    self.id = id
  }
}

public struct WarpnetReportEvent: Encodable, Equatable, Sendable {
  public var type: Int
  public var objectId: String
  public var targetUserId: String
  public var targetNodeId: String
  public var reason: String

  enum CodingKeys: String, CodingKey {
    case type
    case objectId = "object_id"
    case targetUserId = "target_user_id"
    case targetNodeId = "target_node_id"
    case reason
  }

  public init(type: Int, objectId: String = "", targetUserId: String, targetNodeId: String, reason: String) {
    self.type = type
    self.objectId = objectId
    self.targetUserId = targetUserId
    self.targetNodeId = targetNodeId
    self.reason = reason
  }
}

public struct DeleteDeviceEvent: Encodable, Equatable, Sendable {
  public var nodeId: String

  enum CodingKeys: String, CodingKey {
    case nodeId = "node_id"
  }

  public init(nodeId: String) {
    self.nodeId = nodeId
  }
}
