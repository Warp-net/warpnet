import Foundation
import WarpnetTransport

struct ViewerState: Sendable {
  var stats: TweetStatsResponse?
  var poll: PollResultsResponse?
  var isBookmarked = false
  var isRetweeted = false
}

enum WarpnetMapper {
  static func account(_ user: WarpnetUser) -> MastodonAccount {
    let website = user.website?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
    let fields = website.isEmpty ? [] : [
      MastodonField(name: "Website", value: WarpnetFormat.html(fromPlainText: website), verifiedAt: nil),
    ]
    return MastodonAccount(id: user.id,
                           username: user.id,
                           displayName: user.username,
                           avatar: WarpnetFormat.imageURL(userId: user.id, key: user.avatarKey)
                             ?? WarpnetFormat.missingImageURL(userId: user.id),
                           header: WarpnetFormat.imageURL(userId: user.id, key: user.backgroundImageKey)
                             ?? WarpnetFormat.missingImageURL(userId: user.id),
                           acct: user.id,
                           note: WarpnetFormat.html(fromPlainText: user.bio),
                           createdAt: WarpnetFormat.serverDate(fromRFC3339: user.createdAt),
                           followersCount: user.followersCount,
                           followingCount: user.followingsCount,
                           statusesCount: user.tweetsCount,
                           fields: fields,
                           locked: user.locked,
                           emojis: [],
                           url: "\(WarpnetFormat.baseURL)/users/\(WarpnetFormat.pathComponent(user.id))",
                           source: .init(privacy: "public", sensitive: false, note: user.bio, fields: []),
                           bot: false,
                           discoverable: true)
  }

  static func account(userId: String, username: String) -> MastodonAccount {
    account(WarpnetUser(id: userId, username: username.isEmpty ? userId : username))
  }

  static func address(_ tweet: WarpnetTweet) -> StatusAddress {
    let isQuote = !(tweet.quotedTweetId ?? "").isEmpty
    return StatusAddress(tweetId: tweet.id,
                         userId: tweet.userId,
                         rootId: tweet.rootId,
                         retweetedBy: isQuote ? "" : tweet.retweetedBy ?? "",
                         parentId: tweet.parentId ?? "")
  }

  static func parentAddress(_ tweet: WarpnetTweet) -> StatusAddress? {
    guard let parentId = tweet.parentId, !parentId.isEmpty else { return nil }
    let parentUserId = tweet.parentUserId.flatMap { $0.isEmpty ? nil : $0 } ?? tweet.userId
    let rootId = tweet.rootId.isEmpty ? parentId : tweet.rootId
    return StatusAddress(tweetId: parentId,
                         userId: parentUserId,
                         rootId: parentId == rootId ? "" : rootId,
                         parentId: parentId == rootId ? "" : rootId)
  }

  static func status(_ tweet: WarpnetTweet,
                     author: WarpnetUser?,
                     viewer: ViewerState,
                     quote: String? = nil) -> MastodonStatus
  {
    let address = address(tweet).original
    let parent = parentAddress(tweet)
    let stats = viewer.stats
    var content = WarpnetFormat.html(fromPlainText: tweet.text)
    if let quote {
      content += "<a href=\"\(quote)\"></a>"
    }
    return MastodonStatus(id: address.id,
                          content: content,
                          account: author.map { account($0) } ?? account(userId: tweet.userId, username: tweet.username),
                          createdAt: WarpnetFormat.serverDate(fromRFC3339: tweet.createdAt),
                          editedAt: editedAt(tweet),
                          reblog: nil,
                          mediaAttachments: attachments(tweet),
                          repliesCount: stats?.repliesCount ?? 0,
                          reblogsCount: stats?.retweetsCount ?? 0,
                          favouritesCount: stats?.reactionsCount ?? 0,
                          favourited: !(stats?.myReaction ?? "").isEmpty,
                          reblogged: viewer.isRetweeted,
                          pinned: tweet.pinned,
                          bookmarked: viewer.isBookmarked,
                          url: "\(WarpnetFormat.baseURL)/tweets/\(WarpnetFormat.pathComponent(address.tweetId))",
                          inReplyToId: parent?.id,
                          inReplyToAccountId: parent?.userId,
                          poll: poll(tweet, address: address, results: viewer.poll),
                          myReaction: stats.flatMap { $0.myReaction.isEmpty ? nil : $0.myReaction })
  }

  static func retweet(_ original: MastodonStatus, address: StatusAddress, retweeter: MastodonAccount) -> MastodonStatus {
    MastodonStatus(id: address.id,
                   content: "",
                   account: retweeter,
                   createdAt: original.createdAt,
                   editedAt: nil,
                   reblog: original,
                   mediaAttachments: [],
                   repliesCount: 0,
                   reblogsCount: 0,
                   favouritesCount: 0,
                   favourited: false,
                   reblogged: false,
                   pinned: false,
                   bookmarked: false,
                   url: original.url,
                   inReplyToId: nil,
                   inReplyToAccountId: nil,
                   poll: nil)
  }

  static func conversation(_ chat: WarpnetChat, viewerId: String, other: WarpnetUser?) -> MastodonConversation {
    let otherId = chat.ownerId == viewerId ? chat.otherUserId : chat.ownerId
    let party = other.map { account($0) } ?? account(userId: otherId, username: "")
    let lastStatus = chat.lastMessage.isEmpty ? nil : plainStatus(id: "\(chat.id)#last",
                                                                  text: chat.lastMessage,
                                                                  account: party,
                                                                  createdAt: chat.updatedAt.isEmpty ? chat.createdAt : chat.updatedAt)
    return MastodonConversation(id: chat.id, unread: false, lastStatus: lastStatus, accounts: [party])
  }

  static func message(_ message: WarpnetMessage, sender: WarpnetUser?) -> MastodonStatus {
    plainStatus(id: message.id,
                text: message.text,
                account: sender.map { account($0) } ?? account(userId: message.senderId, username: ""),
                createdAt: message.createdAt)
  }

  private static func plainStatus(id: String, text: String, account: MastodonAccount, createdAt: String) -> MastodonStatus {
    MastodonStatus(id: id,
                   content: WarpnetFormat.html(fromPlainText: text),
                   account: account,
                   createdAt: WarpnetFormat.serverDate(fromRFC3339: createdAt),
                   editedAt: nil,
                   reblog: nil,
                   mediaAttachments: [],
                   repliesCount: 0,
                   reblogsCount: 0,
                   favouritesCount: 0,
                   favourited: false,
                   reblogged: false,
                   pinned: false,
                   bookmarked: false,
                   url: "",
                   inReplyToId: nil,
                   inReplyToAccountId: nil,
                   poll: nil)
  }

  static func relationship(userId: String,
                           following: Bool,
                           followedBy: Bool,
                           blocking: Bool = false,
                           muting: Bool = false,
                           requested: Bool = false) -> MastodonRelationship
  {
    MastodonRelationship(id: userId,
                         following: following,
                         followedBy: followedBy,
                         blocking: blocking,
                         muting: muting,
                         requested: requested)
  }

  static func instance() -> MastodonInstance {
    MastodonInstance(title: "Warpnet",
                     shortDescription: "",
                     email: "",
                     version: "4.0.0",
                     stats: .init(userCount: 0, statusCount: 0, domainCount: 0),
                     languages: ["en"],
                     registrations: false,
                     configuration: .init(statuses: .init(maxCharacters: WarpnetLimits.maxTweetBytes,
                                                          maxMediaAttachments: WarpnetLimits.maxImagesPerTweet),
                                          polls: .init(maxOptions: WarpnetLimits.maxPollOptions,
                                                       maxCharactersPerOption: WarpnetLimits.maxPollOptionChars,
                                                       minExpiration: 300,
                                                       maxExpiration: 2_629_746)))
  }

  private static func editedAt(_ tweet: WarpnetTweet) -> String? {
    guard let updated = WarpnetFormat.date(fromRFC3339: tweet.updatedAt),
          let created = WarpnetFormat.date(fromRFC3339: tweet.createdAt),
          updated.timeIntervalSince(created) > 1
    else { return nil }
    return WarpnetFormat.serverDate(updated)
  }

  private static func attachments(_ tweet: WarpnetTweet) -> [MastodonMediaAttachment] {
    var media: [MastodonMediaAttachment] = (tweet.imageKeys ?? []).compactMap { key in
      guard let url = WarpnetFormat.imageURL(userId: tweet.userId, key: key) else { return nil }
      return MastodonMediaAttachment(id: key, type: "image", url: url, previewUrl: url, description: nil)
    }
    if let url = WarpnetFormat.videoURL(userId: tweet.userId, key: tweet.videoKey), let key = tweet.videoKey {
      media.append(MastodonMediaAttachment(id: key, type: "video", url: url, previewUrl: nil, description: nil))
    }
    return media
  }

  private static func poll(_ tweet: WarpnetTweet, address: StatusAddress, results: PollResultsResponse?) -> MastodonPoll? {
    guard let poll = tweet.poll, !poll.options.isEmpty else { return nil }
    let expiresAt = WarpnetFormat.date(fromRFC3339: poll.expiresAt)
    let votes = results?.votes ?? []
    return MastodonPoll(id: address.id,
                        expiresAt: expiresAt.map { WarpnetFormat.serverDate($0) },
                        expired: expiresAt.map { $0 < Date() } ?? false,
                        multiple: false,
                        votesCount: results?.totalVotes ?? 0,
                        votersCount: results?.totalVotes,
                        voted: results?.votedOption != nil,
                        ownVotes: results?.votedOption.map { [$0] } ?? [],
                        options: poll.options.enumerated().map { index, title in
                          MastodonPoll.Option(title: title, votesCount: index < votes.count ? votes[index] : 0)
                        })
  }
}
