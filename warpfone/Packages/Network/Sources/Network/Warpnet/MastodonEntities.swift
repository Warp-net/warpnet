import Foundation

final class MastodonStatus: Encodable, @unchecked Sendable {
  let id: String
  let content: String
  let account: MastodonAccount
  let createdAt: String
  let editedAt: String?
  let reblog: MastodonStatus?
  let mediaAttachments: [MastodonMediaAttachment]
  let mentions: [MastodonMention]
  let repliesCount: Int
  let reblogsCount: Int
  let favouritesCount: Int
  let favourited: Bool
  let myReaction: String?
  let reblogged: Bool
  let pinned: Bool
  let bookmarked: Bool
  let emojis: [MastodonEmoji]
  let url: String
  let inReplyToId: String?
  let inReplyToAccountId: String?
  let visibility: String
  let poll: MastodonPoll?
  let spoilerText: String
  let sensitive: Bool

  init(id: String,
       content: String,
       account: MastodonAccount,
       createdAt: String,
       editedAt: String?,
       reblog: MastodonStatus?,
       mediaAttachments: [MastodonMediaAttachment],
       repliesCount: Int,
       reblogsCount: Int,
       favouritesCount: Int,
       favourited: Bool,
       reblogged: Bool,
       pinned: Bool,
       bookmarked: Bool,
       url: String,
       inReplyToId: String?,
       inReplyToAccountId: String?,
       poll: MastodonPoll?,
       myReaction: String? = nil)
  {
    self.id = id
    self.content = content
    self.account = account
    self.createdAt = createdAt
    self.editedAt = editedAt
    self.reblog = reblog
    self.mediaAttachments = mediaAttachments
    mentions = []
    self.repliesCount = repliesCount
    self.reblogsCount = reblogsCount
    self.favouritesCount = favouritesCount
    self.favourited = favourited
    self.myReaction = myReaction
    self.reblogged = reblogged
    self.pinned = pinned
    self.bookmarked = bookmarked
    emojis = []
    self.url = url
    self.inReplyToId = inReplyToId
    self.inReplyToAccountId = inReplyToAccountId
    visibility = "public"
    self.poll = poll
    spoilerText = ""
    sensitive = false
  }
}

struct MastodonAccount: Encodable, Sendable {
  struct Source: Encodable, Sendable {
    let privacy: String
    let sensitive: Bool
    let note: String
    let fields: [MastodonField]
  }

  let id: String
  let username: String
  let displayName: String
  let avatar: String
  let header: String
  let acct: String
  let note: String
  let createdAt: String
  let followersCount: Int
  let followingCount: Int
  let statusesCount: Int
  let fields: [MastodonField]
  let locked: Bool
  let emojis: [MastodonEmoji]
  let url: String
  let source: Source
  let bot: Bool
  let discoverable: Bool
}

struct MastodonField: Encodable, Sendable {
  let name: String
  let value: String
  let verifiedAt: String?
}

struct MastodonEmoji: Encodable, Sendable {
  let shortcode: String
  let url: String
  let staticUrl: String
  let visibleInPicker: Bool
}

struct MastodonMention: Encodable, Sendable {
  let id: String
  let username: String
  let url: String
  let acct: String
}

struct MastodonMediaAttachment: Encodable, Sendable {
  let id: String
  let type: String
  let url: String
  let previewUrl: String?
  let description: String?
}

struct MastodonPoll: Encodable, Sendable {
  struct Option: Encodable, Sendable {
    let title: String
    let votesCount: Int?
  }

  let id: String
  let expiresAt: String?
  let expired: Bool
  let multiple: Bool
  let votesCount: Int
  let votersCount: Int?
  let voted: Bool
  let ownVotes: [Int]
  let options: [Option]

  enum CodingKeys: String, CodingKey {
    case id, expiresAt, expired, multiple, votesCount, votersCount, voted, ownVotes, options
  }

  func encode(to encoder: Encoder) throws {
    var container = encoder.container(keyedBy: CodingKeys.self)
    try container.encode(id, forKey: .id)
    if let expiresAt {
      try container.encode(expiresAt, forKey: .expiresAt)
    } else {
      try container.encodeNil(forKey: .expiresAt)
    }
    try container.encode(expired, forKey: .expired)
    try container.encode(multiple, forKey: .multiple)
    try container.encode(votesCount, forKey: .votesCount)
    try container.encodeIfPresent(votersCount, forKey: .votersCount)
    try container.encode(voted, forKey: .voted)
    try container.encode(ownVotes, forKey: .ownVotes)
    try container.encode(options, forKey: .options)
  }
}

struct MastodonRelationship: Encodable, Sendable {
  let id: String
  let following: Bool
  let showingReblogs: Bool
  let followedBy: Bool
  let blocking: Bool
  let blockedBy: Bool
  let muting: Bool
  let mutingNotifications: Bool
  let requested: Bool
  let domainBlocking: Bool
  let endorsed: Bool
  let note: String
  let notifying: Bool

  init(id: String,
       following: Bool = false,
       followedBy: Bool = false,
       blocking: Bool = false,
       muting: Bool = false,
       requested: Bool = false,
       notifying: Bool = false)
  {
    self.id = id
    self.following = following
    showingReblogs = true
    self.followedBy = followedBy
    self.blocking = blocking
    blockedBy = false
    self.muting = muting
    mutingNotifications = muting
    self.requested = requested
    domainBlocking = false
    endorsed = false
    note = ""
    self.notifying = notifying
  }
}

struct MastodonNotification: Encodable, Sendable {
  let id: String
  let type: String
  let createdAt: String
  let account: MastodonAccount
  let status: MastodonStatus?
}

struct MastodonContext: Encodable, Sendable {
  let ancestors: [MastodonStatus]
  let descendants: [MastodonStatus]
}

struct MastodonSearchResults: Encodable, Sendable {
  let accounts: [MastodonAccount]
  let statuses: [MastodonStatus]
  let hashtags: [MastodonTag]
}

struct MastodonTag: Encodable, Sendable {
  struct History: Encodable, Sendable {
    let day: String
    let accounts: String
    let uses: String
  }

  let name: String
  let url: String
  let following: Bool
  let history: [History]
}

struct MastodonInstance: Encodable, Sendable {
  struct Stats: Encodable, Sendable {
    let userCount: Int
    let statusCount: Int
    let domainCount: Int
  }

  struct Configuration: Encodable, Sendable {
    struct Statuses: Encodable, Sendable {
      let maxCharacters: Int
      let maxMediaAttachments: Int
    }

    struct Polls: Encodable, Sendable {
      let maxOptions: Int
      let maxCharactersPerOption: Int
      let minExpiration: Int
      let maxExpiration: Int
    }

    let statuses: Statuses
    let polls: Polls
  }

  let title: String
  let shortDescription: String
  let email: String
  let version: String
  let stats: Stats
  let languages: [String]
  let registrations: Bool
  let configuration: Configuration
}

struct MastodonIdentifier: Encodable, Sendable {
  let id: String
}

enum MastodonJSON {
  static func encode<T: Encodable>(_ value: T) throws -> Data {
    let encoder = JSONEncoder()
    encoder.keyEncodingStrategy = .convertToSnakeCase
    encoder.outputFormatting = [.withoutEscapingSlashes]
    return try encoder.encode(value)
  }
}
