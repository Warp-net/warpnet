// Modified by Warpnet. See NOTICE.

import Foundation

public protocol AnyStatus {
  var id: String { get }
  var content: HTMLString { get }
  var account: Account { get }
  var createdAt: ServerDate { get }
  var editedAt: ServerDate? { get }
  var mediaAttachments: [MediaAttachment] { get }
  var mentions: [Mention] { get }
  var repliesCount: Int { get }
  var reblogsCount: Int { get }
  var favouritesCount: Int { get }
  var favourited: Bool? { get }
  var myReaction: String? { get }
  var viewsCount: Int? { get }
  var reactions: [String: Int]? { get }
  var reblogged: Bool? { get }
  var pinned: Bool? { get }
  var bookmarked: Bool? { get }
  var emojis: [Emoji] { get }
  var url: String? { get }
  var inReplyToId: String? { get }
  var inReplyToAccountId: String? { get }
  var poll: Poll? { get }
  var filtered: [Filtered]? { get }
  var isHidden: Bool { get }
}

public final class Status: AnyStatus, Codable, Identifiable, Equatable, Hashable {
  public static func == (lhs: Status, rhs: Status) -> Bool {
    lhs.id == rhs.id && lhs.editedAt?.asDate == rhs.editedAt?.asDate
  }

  public func hash(into hasher: inout Hasher) {
    hasher.combine(id)
  }

  public let id: String
  public let content: HTMLString
  public let account: Account
  public let createdAt: ServerDate
  public let editedAt: ServerDate?
  public let reblog: ReblogStatus?
  public let mediaAttachments: [MediaAttachment]
  public let mentions: [Mention]
  public let repliesCount: Int
  public let reblogsCount: Int
  public let favouritesCount: Int
  public let favourited: Bool?
  public let myReaction: String?
  public let viewsCount: Int?
  public let reactions: [String: Int]?
  public let reblogged: Bool?
  public let pinned: Bool?
  public let bookmarked: Bool?
  public let emojis: [Emoji]
  public let url: String?
  public let inReplyToId: String?
  public let inReplyToAccountId: String?
  public let poll: Poll?
  public let filtered: [Filtered]?
  
  public var isHidden: Bool {
    filtered?.first?.filter.filterAction == .hide
  }

  public init(id: String, content: HTMLString, account: Account, createdAt: ServerDate, editedAt: ServerDate?, reblog: ReblogStatus?, mediaAttachments: [MediaAttachment], mentions: [Mention], repliesCount: Int, reblogsCount: Int, favouritesCount: Int, favourited: Bool?, reblogged: Bool?, pinned: Bool?, bookmarked: Bool?, emojis: [Emoji], url: String?, inReplyToId: String?, inReplyToAccountId: String?, poll: Poll?, filtered: [Filtered]?, myReaction: String? = nil, viewsCount: Int? = nil, reactions: [String: Int]? = nil) {
    self.id = id
    self.content = content
    self.account = account
    self.createdAt = createdAt
    self.editedAt = editedAt
    self.reblog = reblog
    self.mediaAttachments = mediaAttachments
    self.mentions = mentions
    self.repliesCount = repliesCount
    self.reblogsCount = reblogsCount
    self.favouritesCount = favouritesCount
    self.favourited = favourited
    self.myReaction = myReaction
    self.viewsCount = viewsCount
    self.reactions = reactions
    self.reblogged = reblogged
    self.pinned = pinned
    self.bookmarked = bookmarked
    self.emojis = emojis
    self.url = url
    self.inReplyToId = inReplyToId
    self.inReplyToAccountId = inReplyToAccountId
    self.poll = poll
    self.filtered = filtered
  }

  public static func placeholder(forSettings: Bool = false) -> Status {
    .init(id: UUID().uuidString,
          content: .init(stringValue: "Here's to the [#crazy](#) ones. The misfits.\nThe [@rebels](#). The troublemakers.",
                         parseMarkdown: forSettings),

          account: .placeholder(),
          createdAt: ServerDate(),
          editedAt: nil,
          reblog: nil,
          mediaAttachments: [],
          mentions: [],
          repliesCount: 34,
          reblogsCount: 8,
          favouritesCount: 150,
          favourited: false,
          reblogged: false,
          pinned: false,
          bookmarked: false,
          emojis: [],
          url: "https://example.com",
          inReplyToId: nil,
          inReplyToAccountId: nil,
          poll: nil,
          filtered: [])
  }

  public static func placeholders() -> [Status] {
    [.placeholder(), .placeholder(), .placeholder(), .placeholder(), .placeholder(),
     .placeholder(), .placeholder(), .placeholder(), .placeholder(), .placeholder()]
  }

  public var reblogAsAsStatus: Status? {
    if let reblog {
      return .init(id: reblog.id,
                   content: reblog.content,
                   account: reblog.account,
                   createdAt: reblog.createdAt,
                   editedAt: reblog.editedAt,
                   reblog: nil,
                   mediaAttachments: reblog.mediaAttachments,
                   mentions: reblog.mentions,
                   repliesCount: reblog.repliesCount,
                   reblogsCount: reblog.reblogsCount,
                   favouritesCount: reblog.favouritesCount,
                   favourited: reblog.favourited,
                   reblogged: reblog.reblogged,
                   pinned: reblog.pinned,
                   bookmarked: reblog.bookmarked,
                   emojis: reblog.emojis,
                   url: reblog.url,
                   inReplyToId: reblog.inReplyToId,
                   inReplyToAccountId: reblog.inReplyToAccountId,
                   poll: reblog.poll,
                   filtered: reblog.filtered,
                   myReaction: reblog.myReaction,
                   viewsCount: reblog.viewsCount,
                   reactions: reblog.reactions)
    }
    return nil
  }
}

public final class ReblogStatus: AnyStatus, Codable, Identifiable, Equatable, Hashable {
  public static func == (lhs: ReblogStatus, rhs: ReblogStatus) -> Bool {
    lhs.id == rhs.id
  }

  public func hash(into hasher: inout Hasher) {
    hasher.combine(id)
  }

  public let id: String
  public let content: HTMLString
  public let account: Account
  public let createdAt: ServerDate
  public let editedAt: ServerDate?
  public let mediaAttachments: [MediaAttachment]
  public let mentions: [Mention]
  public let repliesCount: Int
  public let reblogsCount: Int
  public let favouritesCount: Int
  public let favourited: Bool?
  public let myReaction: String?
  public let viewsCount: Int?
  public let reactions: [String: Int]?
  public let reblogged: Bool?
  public let pinned: Bool?
  public let bookmarked: Bool?
  public let emojis: [Emoji]
  public let url: String?
  public let inReplyToId: String?
  public let inReplyToAccountId: String?
  public let poll: Poll?
  public let filtered: [Filtered]?
  
  public var isHidden: Bool {
    filtered?.first?.filter.filterAction == .hide
  }

  public init(id: String, content: HTMLString, account: Account, createdAt: ServerDate, editedAt: ServerDate?, mediaAttachments: [MediaAttachment], mentions: [Mention], repliesCount: Int, reblogsCount: Int, favouritesCount: Int, favourited: Bool?, reblogged: Bool?, pinned: Bool?, bookmarked: Bool?, emojis: [Emoji], url: String?, inReplyToId: String?, inReplyToAccountId: String?, poll: Poll?, filtered: [Filtered]?, myReaction: String? = nil, viewsCount: Int? = nil, reactions: [String: Int]? = nil) {
    self.id = id
    self.content = content
    self.account = account
    self.createdAt = createdAt
    self.editedAt = editedAt
    self.mediaAttachments = mediaAttachments
    self.mentions = mentions
    self.repliesCount = repliesCount
    self.reblogsCount = reblogsCount
    self.favouritesCount = favouritesCount
    self.favourited = favourited
    self.myReaction = myReaction
    self.viewsCount = viewsCount
    self.reactions = reactions
    self.reblogged = reblogged
    self.pinned = pinned
    self.bookmarked = bookmarked
    self.emojis = emojis
    self.url = url
    self.inReplyToId = inReplyToId
    self.inReplyToAccountId = inReplyToAccountId
    self.poll = poll
    self.filtered = filtered
  }
}

// Every property in Status is immutable.
extension Status: Sendable {}

// Every property in ReblogStatus is immutable.
extension ReblogStatus: Sendable {}
