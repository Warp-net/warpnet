import Foundation
import Models
import WarpnetTransport

struct WarpnetRouter: Sendable {
  enum Method: String, Sendable {
    case get, post, put, patch, delete
  }

  struct Response: Sendable {
    let body: Data
    let next: String?
  }

  static let minimumPage = 20
  static let maxPagesPerRequest = 5
  static let maxResumePages = 10
  static let maxListPages = 10
  static let defaultReaction = "❤️"
  static let systemAccountId = "warpnet"
  static let userReport = 0
  static let tweetReport = 1

  let repository: WarpnetRepository

  init(session: WarpnetSession) {
    repository = WarpnetRepository(session: session)
  }

  func route(_ endpoint: Endpoint, method: Method) async throws -> Response {
    switch endpoint {
    case let endpoint as Timelines:
      return try await timelines(endpoint)
    case let endpoint as Statuses:
      return try await statuses(endpoint, method: method)
    case let endpoint as Accounts:
      return try await accounts(endpoint, method: method)
    case let endpoint as Notifications:
      return try await notifications(endpoint)
    case let endpoint as Search:
      return try await search(endpoint)
    case let endpoint as Polls:
      return try await polls(endpoint)
    case let endpoint as FollowRequests:
      return try await followRequests(endpoint)
    case let endpoint as Markers:
      return try await markers(endpoint)
    case let endpoint as Instances:
      return try await instances(endpoint)
    case let endpoint as Chats:
      return try await chats(endpoint)
    case let endpoint as ServerFilters:
      return try await filters(endpoint, method: method)
    case let endpoint as TweetViews:
      return try await views(endpoint)
    default:
      throw WarpnetRoutingError.unsupported(endpoint.path())
    }
  }

  func upload(_ endpoint: Endpoint, mimeType: String, filename: String, data: Data) async throws -> Data {
    if let endpoint = endpoint as? Accounts, case .updateCredentialsMedia = endpoint {
      return try await uploadProfileImage(data, mimeType: mimeType, isHeader: filename == "header")
    }
    guard let endpoint = endpoint as? Media, case .medias = endpoint else {
      throw WarpnetRoutingError.unsupported(endpoint.path())
    }
    let pairing = try await repository.viewer()
    if mimeType.lowercased().hasPrefix("video/") {
      guard WarpnetLimits.acceptedVideoMimeTypes.contains(mimeType.lowercased()) else {
        throw WarpnetRoutingError.invalidMedia("Only MP4, QuickTime and M4V videos are accepted")
      }
      guard data.count <= WarpnetLimits.maxVideoBytes else {
        throw WarpnetRoutingError.invalidMedia("The video is larger than 36 MiB")
      }
      let key = try await repository.uploadVideo(data, mimeType: mimeType)
      return try encode(mediaAttachment(id: WarpnetRepository.videoPrefix + key, ownerId: pairing.userId))
    }
    guard mimeType.lowercased().hasPrefix("image/") else {
      throw WarpnetRoutingError.invalidMedia("Only images and videos can be attached")
    }
    let key = try await repository.uploadImage(data, mimeType: mimeType)
    return try encode(mediaAttachment(id: WarpnetRepository.imagePrefix + key, ownerId: pairing.userId))
  }

  func pushes(after lastSeenId: String?) async throws -> Data {
    let pairing = try await repository.viewer()
    let page: GetNotificationsResponse = try await repository.call(ProtocolIds.PRIVATE_GET_PUSHES, GetNotificationsEvent(limit: 20))
    let fresh = page.notifications.filter { item in lastSeenId.map { item.id > $0 } ?? true }
    return try await encode(notifications(fresh, viewerId: pairing.userId))
  }

  func mediaData(for url: URL) async throws -> Data {
    guard url.scheme == "warpnet", let kind = url.host() else { throw WarpnetRoutingError.invalidMedia(url.absoluteString) }
    let parts = url.pathComponents.filter { $0 != "/" }
    guard parts.count == 2 else { throw WarpnetRoutingError.invalidMedia(url.absoluteString) }
    switch kind {
    case "image":
      guard parts[1] != "missing.png" else { throw WarpnetRoutingError.invalidMedia(url.absoluteString) }
      return try await repository.imageData(userId: parts[0], key: parts[1])
    case "video":
      return try await repository.videoData(userId: parts[0], key: parts[1])
    default:
      throw WarpnetRoutingError.invalidMedia(url.absoluteString)
    }
  }

  // MARK: - Timelines

  private func timelines(_ endpoint: Timelines) async throws -> Response {
    switch endpoint {
    case let .home(sinceId, maxId, minId):
      let repository = repository
      return try await pagedStatuses(scope: "home", maxId: maxId, newerThan: minId ?? sinceId, filterContext: "home", fetch: { cursor in
        try await repository.homePage(cursor: cursor)
      })
    }
  }

  // MARK: - Statuses

  private func statuses(_ endpoint: Statuses, method: Method) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case let .status(id):
      let address = try await repository.address(of: id)
      if method == .delete {
        try await delete(address, viewerId: pairing.userId)
        return try response(MastodonIdentifier(id: id))
      }
      return try await response(repository.status(at: address))

    case let .context(id):
      let address = try await repository.address(of: id)
      let tweet = try await repository.tweet(address.original)
      async let ancestors = repository.ancestors(of: tweet)
      async let descendants = repository.descendants(of: address.original)
      let (up, down) = await (ancestors, descendants)
      async let ancestorStatuses = repository.statuses(up, filterContext: "thread")
      async let descendantStatuses = repository.statuses(down, filterContext: "thread")
      return try await response(MastodonContext(ancestors: ancestorStatuses, descendants: descendantStatuses))

    case let .postStatus(json):
      return try await response(post(json, viewer: pairing))

    case let .editStatus(id, json):
      let address = try await repository.address(of: id)
      let edited: WarpnetTweet = try await repository.call(ProtocolIds.PRIVATE_POST_TWEET_EDIT,
                                                           EditTweetEvent(tweetId: address.tweetId,
                                                                          userId: pairing.userId,
                                                                          text: json.status))
      await repository.forgetTweet(address.tweetId)
      guard let status = await repository.status(edited) else { throw WarpnetRoutingError.unknownStatus(id) }
      return try response(status)

    case let .favorite(id):
      return try await react(id, emoji: "", viewerId: pairing.userId)

    case let .react(id, emoji):
      return try await react(id, emoji: emoji, viewerId: pairing.userId)

    case let .unfavorite(id):
      return try await unreact(id, viewerId: pairing.userId)

    case let .reblog(id):
      let address = try await repository.address(of: id).original
      let tweet = try await repository.tweet(address)
      let author = try? await repository.user(address.userId)
      try await repository.send(ProtocolIds.PUBLIC_POST_RETWEET,
                                WarpnetTweet(id: address.tweetId,
                                             createdAt: WarpnetTimestamp.rfc3339(nanos: WarpnetTimestamp.nowNanos()),
                                             retweetedBy: pairing.userId,
                                             rootId: "",
                                             text: tweet.text,
                                             userId: address.userId,
                                             username: author?.username ?? tweet.username,
                                             imageKeys: tweet.imageKeys,
                                             videoKey: tweet.videoKey))
      await repository.setRetweeted(true, tweetId: address.tweetId)
      await adjustStats(address.tweetId) { $0.retweetsCount += 1 }
      return try await response(repository.status(at: address))

    case let .unreblog(id):
      let address = try await repository.address(of: id).original
      try await repository.send(ProtocolIds.PUBLIC_POST_UNRETWEET,
                                UnretweetEvent(tweetId: address.tweetId, retweeterId: pairing.userId))
      await repository.setRetweeted(false, tweetId: address.tweetId)
      await adjustStats(address.tweetId) { $0.retweetsCount = max(0, $0.retweetsCount - 1) }
      return try await response(repository.status(at: address))

    case let .rebloggedBy(id, maxId):
      return try await engagement(ProtocolIds.PUBLIC_GET_TWEET_RETWEETERS, id: id, cursor: maxId)

    case let .favoritedBy(id, maxId):
      return try await engagement(ProtocolIds.PUBLIC_GET_TWEET_REACTORS, id: id, cursor: maxId)

    case let .pin(id):
      return try await pin(id, route: ProtocolIds.PUBLIC_POST_PIN, viewerId: pairing.userId)

    case let .unpin(id):
      return try await pin(id, route: ProtocolIds.PUBLIC_POST_UNPIN, viewerId: pairing.userId)

    case let .bookmark(id):
      let address = try await repository.address(of: id).original
      try await repository.send(ProtocolIds.PRIVATE_POST_BOOKMARK,
                                BookmarkEvent(userId: pairing.userId, tweetId: address.tweetId, ownerUserId: address.userId))
      await repository.setBookmarked(true, tweetId: address.tweetId)
      return try await response(repository.status(at: address))

    case let .unbookmark(id):
      let address = try await repository.address(of: id).original
      try await repository.send(ProtocolIds.PRIVATE_POST_UNBOOKMARK,
                                UnbookmarkEvent(userId: pairing.userId, tweetId: address.tweetId))
      await repository.setBookmarked(false, tweetId: address.tweetId)
      return try await response(repository.status(at: address))

    case let .report(accountId, statusId, comment):
      var tweetId: String?
      if let statusId {
        tweetId = try await repository.address(of: statusId).original.tweetId
      }
      let target = try await repository.user(accountId)
      let reason = String(String.UnicodeScalarView(comment.unicodeScalars.prefix(WarpnetLimits.maxReportReasonRunes)))
      try await repository.send(ProtocolIds.PUBLIC_POST_REPORT,
                                WarpnetReportEvent(type: tweetId == nil ? Self.userReport : Self.tweetReport,
                                                   objectId: tweetId ?? "",
                                                   targetUserId: accountId,
                                                   targetNodeId: target.nodeId,
                                                   reason: reason))
      return try response(MastodonIdentifier(id: tweetId ?? accountId))
    }
  }

  private func post(_ json: StatusData, viewer: AuthNodeInfo) async throws -> MastodonStatus {
    let author = try await repository.user(viewer.userId)
    if let quotedStatusId = json.quotedStatusId {
      return try await quote(quotedStatusId, json: json, viewer: viewer, username: author.username)
    }
    var parent: StatusAddress?
    if let inReplyToId = json.inReplyToId {
      parent = try await repository.address(of: inReplyToId).original
    }
    let media = json.mediaIds ?? []
    let images = media.filter { $0.hasPrefix(WarpnetRepository.imagePrefix) }
      .map { String($0.dropFirst(WarpnetRepository.imagePrefix.count)) }
    let video = media.first { $0.hasPrefix(WarpnetRepository.videoPrefix) }
      .map { String($0.dropFirst(WarpnetRepository.videoPrefix.count)) }
    let poll = json.poll.map { poll in
      WarpnetPoll(options: poll.options,
                  expiresAt: WarpnetTimestamp.rfc3339(nanos: (Int64(Date().timeIntervalSince1970) + Int64(poll.expires_in)) * 1_000_000_000))
    }
    let draft = WarpnetTweet(id: "",
                             parentId: parent?.tweetId,
                             parentUserId: parent?.userId,
                             rootId: parent?.threadRootId ?? "",
                             text: json.status,
                             userId: viewer.userId,
                             username: author.username,
                             imageKeys: images.isEmpty ? nil : images,
                             videoKey: video,
                             poll: poll)
    let created: WarpnetTweet = try await repository.call(ProtocolIds.PRIVATE_POST_TWEET, draft)
    guard let status = await repository.status(created) else { throw WarpnetRoutingError.emptyResponse(ProtocolIds.PRIVATE_POST_TWEET) }
    return status
  }

  private func quote(_ id: String, json: StatusData, viewer: AuthNodeInfo, username: String) async throws -> MastodonStatus {
    let source = try await repository.address(of: id).original
    let media = json.mediaIds ?? []
    let images = media.filter { $0.hasPrefix(WarpnetRepository.imagePrefix) }
      .map { String($0.dropFirst(WarpnetRepository.imagePrefix.count)) }
    let draft = WarpnetTweet(id: source.tweetId,
                             createdAt: WarpnetTimestamp.rfc3339(nanos: WarpnetTimestamp.nowNanos()),
                             retweetedBy: viewer.userId,
                             text: json.status.trimmingCharacters(in: .whitespacesAndNewlines),
                             userId: viewer.userId,
                             username: username,
                             imageKeys: images.isEmpty ? nil : images,
                             quotedTweetId: source.tweetId,
                             quotedUserId: source.userId)
    let created: WarpnetTweet = try await repository.call(ProtocolIds.PUBLIC_POST_RETWEET, draft)
    guard let status = await repository.status(created) else { throw WarpnetRoutingError.emptyResponse(ProtocolIds.PUBLIC_POST_RETWEET) }
    return status
  }

  private func pin(_ id: String, route: String, viewerId: String) async throws -> Response {
    let address = try await repository.address(of: id).original
    try await repository.send(route, PinTweetEvent(userId: viewerId, tweetId: address.tweetId))
    await repository.forgetTweet(address.tweetId)
    return try await response(repository.status(at: address))
  }

  private func delete(_ address: StatusAddress, viewerId: String) async throws {
    if address.isRetweet, address.retweetedBy == viewerId {
      try await repository.send(ProtocolIds.PUBLIC_POST_UNRETWEET,
                                UnretweetEvent(tweetId: address.tweetId, retweeterId: viewerId))
      await repository.setRetweeted(false, tweetId: address.tweetId)
      return
    }
    try await repository.send(ProtocolIds.PRIVATE_DELETE_TWEET,
                              DeleteTweetEvent(tweetId: address.tweetId,
                                               userId: viewerId,
                                               parentId: address.parentId,
                                               rootId: address.rootId))
    await repository.forgetTweet(address.tweetId)
  }

  private func react(_ id: String, emoji: String, viewerId: String) async throws -> Response {
    let address = try await repository.address(of: id).original
    let counts: ReactionsCountResponse = try await repository.call(ProtocolIds.PUBLIC_POST_REACT,
                                                                   ReactionEvent(tweetId: address.tweetId,
                                                                                 userId: address.userId,
                                                                                 ownerId: viewerId,
                                                                                 emoji: emoji))
    await adjustStats(address.tweetId) { stats in
      stats.reactionsCount = counts.reactionsCount
      stats.reactions = counts.reactions
      stats.myReaction = emoji.isEmpty ? Self.defaultReaction : emoji
    }
    return try await response(repository.status(at: address))
  }

  private func unreact(_ id: String, viewerId: String) async throws -> Response {
    let address = try await repository.address(of: id).original
    let counts: ReactionsCountResponse = try await repository.call(ProtocolIds.PUBLIC_POST_UNREACT,
                                                                   ReactionEvent(tweetId: address.tweetId,
                                                                                 userId: address.userId,
                                                                                 ownerId: viewerId))
    await adjustStats(address.tweetId) { stats in
      stats.reactionsCount = counts.reactionsCount
      stats.reactions = counts.reactions
      stats.myReaction = ""
    }
    return try await response(repository.status(at: address))
  }

  private func adjustStats(_ tweetId: String, _ change: (inout TweetStatsResponse) -> Void) async {
    var stats = await repository.cachedStats(tweetId) ?? TweetStatsResponse(tweetId: tweetId)
    change(&stats)
    await repository.setStats(stats, for: tweetId)
  }

  private func engagement(_ route: String, id: String, cursor: String?) async throws -> Response {
    let address = try await repository.address(of: id).original
    let page: UsersResponse = try await repository.call(route,
                                                        GetTweetReactorsEvent(tweetId: address.tweetId,
                                                                              ownerUserId: address.userId,
                                                                              cursor: cursor ?? "",
                                                                              limit: WarpnetRepository.pageSize))
    return try response(page.users.map { WarpnetMapper.account($0) }, next: nextToken(page.cursor, count: page.users.count))
  }

  // MARK: - Accounts

  private func accounts(_ endpoint: Accounts, method _: Method) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case .verifyCredentials:
      return try await response(WarpnetMapper.account(repository.user(pairing.userId)))

    case let .accounts(id):
      return try await response(WarpnetMapper.account(repository.user(id)))

    case let .relationships(ids):
      async let blocked = listedIds(ProtocolIds.PRIVATE_GET_SETTINGS_BLOCKS, viewerId: pairing.userId)
      async let muted = listedIds(ProtocolIds.PRIVATE_GET_SETTINGS_MUTES, viewerId: pairing.userId)
      let (blockedIds, mutedIds) = await (blocked, muted)
      let relationships = await repository.concurrentMap(ids) { id in
        try? await relationship(with: id,
                                viewerId: pairing.userId,
                                blocking: blockedIds.contains(id),
                                muting: mutedIds.contains(id))
      }
      return try response(relationships.compactMap { $0 })

    case let .follow(id):
      try await repository.send(ProtocolIds.PUBLIC_POST_FOLLOW, NewFollowEvent(followerId: pairing.userId, followeeId: id))
      await repository.forgetUser(id)
      return try await response(relationship(with: id, viewerId: pairing.userId))

    case let .unfollow(id):
      try await repository.send(ProtocolIds.PUBLIC_POST_UNFOLLOW, NewUnfollowEvent(followerId: pairing.userId, followeeId: id))
      await repository.forgetUser(id)
      return try await response(relationship(with: id, viewerId: pairing.userId))

    case let .followers(id, maxId):
      let page: FollowersResponse = try await repository.call(ProtocolIds.PUBLIC_GET_FOLLOWERS,
                                                              GetFollowersEvent(userId: id, cursor: maxId ?? "", limit: WarpnetRepository.pageSize))
      return try await response(repository.accounts(page.followers), next: nextToken(page.cursor, count: page.followers.count))

    case let .following(id, maxId):
      let page: FollowingsResponse = try await repository.call(ProtocolIds.PUBLIC_GET_FOLLOWINGS,
                                                               GetFollowingsEvent(userId: id, cursor: maxId ?? "", limit: WarpnetRepository.pageSize))
      return try await response(repository.accounts(page.followings), next: nextToken(page.cursor, count: page.followings.count))

    case let .statuses(id, sinceId, onlyMedia, excludeReplies, excludeReblogs, pinned):
      let repository = repository
      if pinned == true {
        let page = try await repository.userPage(userId: id, cursor: "")
        return try await response(repository.statuses(page.tweets.filter(\.pinned)))
      }
      let scope = "user|\(id)|\(onlyMedia)|\(excludeReplies)|\(excludeReblogs)"
      return try await pagedStatuses(scope: scope, maxId: sinceId, newerThan: nil, keep: { tweet in
        let address = WarpnetMapper.address(tweet)
        let hasMedia = !(tweet.imageKeys ?? []).isEmpty || !(tweet.videoKey ?? "").isEmpty
        return (!onlyMedia || hasMedia) && (!excludeReblogs || !address.isRetweet) && (!excludeReplies || address.parentId.isEmpty)
      }, fetch: { cursor in
        try await repository.userPage(userId: id, cursor: cursor)
      })

    case let .favorites(sinceId):
      return try await shelf(ProtocolIds.PRIVATE_GET_REACTIONS, viewerId: pairing.userId, cursor: sinceId)

    case let .bookmarks(sinceId):
      return try await shelf(ProtocolIds.PRIVATE_GET_BOOKMARKS, viewerId: pairing.userId, cursor: sinceId)

    case .suggestions:
      let page: UsersResponse = try await repository.call(ProtocolIds.PUBLIC_GET_WHOTOFOLLOW,
                                                          GetAllUsersEvent(userId: pairing.userId, limit: 20))
      return try response(page.users.filter { !$0.isBridged }.map { WarpnetMapper.account($0) })

    case let .block(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_SETTINGS_BLOCK, BlockEvent(blockerId: pairing.userId, blockeeId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId, blocking: true))

    case let .unblock(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_SETTINGS_UNBLOCK, BlockEvent(blockerId: pairing.userId, blockeeId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId, blocking: false))

    case let .mute(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_SETTINGS_MUTE, MuteEvent(muterId: pairing.userId, muteeId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId, muting: true))

    case let .unmute(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_SETTINGS_UNMUTE, MuteEvent(muterId: pairing.userId, muteeId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId, muting: false))

    case let .updateCredentials(json):
      let displayName = json.displayName.trimmingCharacters(in: .whitespacesAndNewlines)
      return try await response(updateProfile(UpdateProfileEvent(username: displayName.isEmpty ? nil : displayName,
                                                                 bio: json.note,
                                                                 website: json.website.trimmingCharacters(in: .whitespacesAndNewlines))))

    case .updateCredentialsMedia:
      throw WarpnetRoutingError.unsupported(endpoint.path())
    }
  }

  private func updateProfile(_ event: UpdateProfileEvent) async throws -> MastodonAccount {
    let pairing = try await repository.viewer()
    let updated: WarpnetUser = try await repository.call(ProtocolIds.PRIVATE_POST_USER, event)
    await repository.forgetUser(pairing.userId)
    return WarpnetMapper.account(updated)
  }

  private func uploadProfileImage(_ data: Data, mimeType: String, isHeader: Bool) async throws -> Data {
    guard mimeType.lowercased().hasPrefix("image/") else {
      throw WarpnetRoutingError.invalidMedia("Only images can be used on a profile")
    }
    let key = try await repository.uploadImage(data, mimeType: mimeType)
    let event = isHeader ? UpdateProfileEvent(backgroundImageKey: key) : UpdateProfileEvent(avatarKey: key)
    return try await encode(updateProfile(event))
  }

  private func relationship(with userId: String,
                            viewerId: String,
                            blocking: Bool? = nil,
                            muting: Bool? = nil) async throws -> MastodonRelationship
  {
    let probe = GetIsFollowingEvent(userId: userId)
    async let following: IsFollowingResponse? = try? repository.call(ProtocolIds.PUBLIC_POST_IS_FOLLOWING, probe)
    async let follower: IsFollowerResponse? = try? repository.call(ProtocolIds.PUBLIC_POST_IS_FOLLOWER, probe)
    async let blocked = blocking == nil ? listedIds(ProtocolIds.PRIVATE_GET_SETTINGS_BLOCKS, viewerId: viewerId) : []
    async let muted = muting == nil ? listedIds(ProtocolIds.PRIVATE_GET_SETTINGS_MUTES, viewerId: viewerId) : []
    let (isFollowing, isFollower) = await (following?.isFollowing ?? false, follower?.isFollower ?? false)
    let (blockedIds, mutedIds) = await (blocked, muted)
    return WarpnetMapper.relationship(userId: userId,
                                      following: isFollowing,
                                      followedBy: isFollower,
                                      blocking: blocking ?? blockedIds.contains(userId),
                                      muting: muting ?? mutedIds.contains(userId))
  }

  private func listedIds(_ route: String, viewerId: String) async -> Set<String> {
    var ids = Set<String>()
    var cursor = ""
    for _ in 0 ..< Self.maxListPages {
      guard let page: GetBlocksResponse = try? await repository.call(route, GetBlocksEvent(userId: viewerId,
                                                                                          cursor: cursor,
                                                                                          limit: WarpnetRepository.pageSize))
      else { break }
      ids.formUnion(page.ids)
      guard !page.ids.isEmpty, !page.cursor.isEmpty, page.cursor != WarpnetRepository.endCursor else { break }
      cursor = page.cursor
    }
    return ids
  }

  private func shelf(_ route: String, viewerId: String, cursor: String?) async throws -> Response {
    let page: GetBookmarksResponse = try await repository.call(route,
                                                              GetBookmarksEvent(userId: viewerId,
                                                                                cursor: cursor ?? "",
                                                                                limit: WarpnetRepository.pageSize))
    let repository = repository
    let tweets = await repository.concurrentMap(page.items) { item in
      try? await repository.tweet(StatusAddress(tweetId: item.tweetId, userId: item.ownerUserId))
    }.compactMap { $0 }
    if route == ProtocolIds.PRIVATE_GET_BOOKMARKS {
      for item in page.items {
        await repository.setBookmarked(true, tweetId: item.tweetId)
      }
    }
    return try await response(repository.statuses(tweets), next: nextToken(page.cursor, count: page.items.count))
  }

  // MARK: - Notifications

  private func notifications(_ endpoint: Notifications) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case let .notifications(minId, maxId, excludedTypes, limit):
      let wanted = max(limit, 20)
      let excluded = Set(excludedTypes ?? [])
      var cursor = ""
      if let maxId {
        guard let known = await repository.cursor(scope: "notifications", after: maxId),
              known != WarpnetRepository.endCursor
        else { return try response([MastodonNotification]()) }
        cursor = known
      }
      var collected: [MastodonNotification] = []
      for _ in 0 ..< Self.maxPagesPerRequest {
        let page: GetNotificationsResponse = try await repository.call(ProtocolIds.PRIVATE_GET_NOTIFICATIONS,
                                                                       GetNotificationsEvent(cursor: cursor, limit: wanted))
        var items = page.notifications
        if let minId {
          if let index = items.firstIndex(where: { $0.id == minId }) {
            items = Array(items[..<index])
          } else {
            items = items.filter { $0.id > minId }
          }
        }
        let mapped = await notifications(items, viewerId: pairing.userId).filter { !excluded.contains($0.type) }
        collected += mapped
        guard minId == nil else { break }
        // Ice Cubes pages from the last id of its regrouped list, which can be
        // any id of the page.
        for item in page.notifications {
          await repository.storeCursor(page.cursor, scope: "notifications", after: item.id)
        }
        guard collected.count < wanted,
              !page.notifications.isEmpty,
              !page.cursor.isEmpty,
              page.cursor != WarpnetRepository.endCursor
        else { break }
        cursor = page.cursor
      }
      return try response(collected)
    }
  }

  private func notifications(_ items: [WarpnetNotification], viewerId: String) async -> [MastodonNotification] {
    let supported = items.compactMap { item in Self.notificationType(item.type).map { (item, $0) } }
    let actors = await repository.users(supported.compactMap { $0.0.actorId })
    let repository = repository
    let statuses = await repository.concurrentMap(supported) { item, type -> MastodonStatus? in
      // A reply notice names the viewer's own tweet, not the reply.
      guard let tweetId = item.tweetId, !tweetId.isEmpty, type != "follow", type != "message", item.type != "reply" else { return nil }
      return try? await repository.status(at: StatusAddress(tweetId: tweetId, userId: viewerId), filterContext: "notifications")
    }
    return zip(supported, statuses).map { entry, status in
      let (item, type) = entry
      let account = item.actorId.flatMap { actors[$0] }.map { WarpnetMapper.account($0) }
        ?? WarpnetMapper.account(userId: item.actorId ?? Self.systemAccountId, username: item.actorId == nil ? "Warpnet" : item.text)
      return MastodonNotification(id: item.id,
                                  type: type,
                                  createdAt: WarpnetFormat.serverDate(fromRFC3339: item.createdAt),
                                  account: account,
                                  status: status,
                                  text: item.text.isEmpty ? nil : item.text)
    }
  }

  private static func notificationType(_ type: String) -> String? {
    switch type {
    case "reaction": "favourite"
    case "retweet": "reblog"
    case "follow": "follow"
    case "mention", "reply": "mention"
    case "message": "message"
    case "moderation": "moderation"
    default: nil
    }
  }

  // MARK: - Views

  private func views(_ endpoint: TweetViews) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case let .record(statusId):
      let address = try await repository.address(of: statusId).original
      guard address.userId != pairing.userId else { return try response(EmptyEvent()) }
      let views: ViewsCountResponse = try await repository.call(ProtocolIds.PUBLIC_POST_VIEW,
                                                                ViewEvent(tweetId: address.tweetId,
                                                                          userId: address.userId,
                                                                          viewerId: pairing.userId))
      await adjustStats(address.tweetId) { $0.viewsCount = views.count }
      return try response(EmptyEvent())
    }
  }

  // MARK: - Filters

  private func filters(_ endpoint: ServerFilters, method: Method) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case .filters:
      let page: GetFiltersResponse = try await repository.call(ProtocolIds.PRIVATE_GET_SETTINGS_FILTERS,
                                                               GetFiltersEvent(userId: pairing.userId, limit: 100))
      await repository.setFilters(page.filters)
      return try response(page.filters.map(WarpnetMapper.serverFilter))

    case let .filter(id) where method == .delete:
      try await repository.send(ProtocolIds.PRIVATE_DELETE_SETTINGS_FILTER, DeleteFilterEvent(userId: pairing.userId, filterId: id))
      await repository.forgetFilters()
      return try response(EmptyEvent())

    case let .filter(id):
      let filter: WarpnetFilter = try await repository.call(ProtocolIds.PRIVATE_GET_SETTINGS_FILTER,
                                                            GetFilterEvent(userId: pairing.userId, filterId: id))
      return try response(WarpnetMapper.serverFilter(filter))

    case let .createFilter(json):
      let created: WarpnetFilter = try await repository.call(ProtocolIds.PRIVATE_POST_SETTINGS_FILTER,
                                                             WarpnetMapper.filter(json, id: "", userId: pairing.userId))
      await repository.forgetFilters()
      return try response(WarpnetMapper.serverFilter(created))

    case let .editFilter(id, json):
      let updated: WarpnetFilter = try await repository.call(ProtocolIds.PRIVATE_POST_SETTINGS_FILTER_UPDATE,
                                                             WarpnetMapper.filter(json, id: id, userId: pairing.userId))
      await repository.forgetFilters()
      return try response(WarpnetMapper.serverFilter(updated))

    case let .addKeyword(filterId, keyword, wholeWord):
      let created: WarpnetFilterKeyword = try await repository.call(ProtocolIds.PRIVATE_POST_SETTINGS_FILTER_KEYWORD,
                                                                    AddFilterKeywordEvent(userId: pairing.userId,
                                                                                          filterId: filterId,
                                                                                          keyword: keyword,
                                                                                          wholeWord: wholeWord))
      await repository.forgetFilters()
      return try response(MastodonServerFilter.Keyword(id: created.id, keyword: created.keyword, wholeWord: created.wholeWord))

    case let .removeKeyword(id):
      try await repository.send(ProtocolIds.PRIVATE_DELETE_SETTINGS_FILTER_KEYWORD,
                                DeleteFilterKeywordEvent(userId: pairing.userId, keywordId: id))
      await repository.forgetFilters()
      return try response(EmptyEvent())
    }
  }

  // MARK: - Chats

  private func chats(_ endpoint: Chats) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case let .chats(cursor):
      if let cursor, Self.isEnd(cursor) {
        return try response([MastodonConversation]())
      }
      let page: GetChatsResponse = try await repository.call(ProtocolIds.PRIVATE_GET_CHATS,
                                                             GetChatsEvent(userId: pairing.userId,
                                                                           cursor: cursor ?? "",
                                                                           limit: WarpnetRepository.pageSize))
      let others = page.chats.map { $0.ownerId == pairing.userId ? $0.otherUserId : $0.ownerId }
      let users = await repository.users(others)
      let conversations = zip(page.chats, others).map { chat, other in
        WarpnetMapper.conversation(chat, viewerId: pairing.userId, other: users[other])
      }
      return try response(conversations, next: nextToken(page.cursor, count: page.chats.count))

    case let .messages(chatId):
      let page: GetMessagesResponse = try await repository.call(ProtocolIds.PRIVATE_GET_MESSAGES,
                                                                GetMessagesEvent(ownerId: pairing.userId,
                                                                                 chatId: chatId,
                                                                                 limit: WarpnetRepository.pageSize))
      let users = await repository.users(Array(Set(page.messages.map(\.senderId))))
      let messages = page.messages.sorted { $0.id < $1.id }
      return try response(messages.map { WarpnetMapper.message($0, sender: users[$0.senderId]) })

    case let .open(accountId):
      let chat: WarpnetChat = try await repository.call(ProtocolIds.PUBLIC_POST_CHAT,
                                                        NewChatEvent(ownerId: pairing.userId, otherUserId: accountId))
      let other = try? await repository.user(accountId)
      return try response(WarpnetMapper.conversation(chat, viewerId: pairing.userId, other: other))

    case let .send(chatId, receiverId, text):
      let message: WarpnetMessage = try await repository.call(ProtocolIds.PUBLIC_POST_MESSAGE,
                                                              NewMessageEvent(chatId: chatId,
                                                                              senderId: pairing.userId,
                                                                              receiverId: receiverId,
                                                                              text: text))
      let sender = try? await repository.user(pairing.userId)
      return try response(WarpnetMapper.message(message, sender: sender))

    case let .delete(chatId):
      try await repository.send(ProtocolIds.PRIVATE_DELETE_CHAT, DeleteChatEvent(userId: pairing.userId, chatId: chatId))
      return try response(EmptyEvent())

    case let .deleteMessage(chatId, messageId):
      try await repository.send(ProtocolIds.PRIVATE_DELETE_MESSAGE, DeleteMessageEvent(chatId: chatId, id: messageId))
      return try response(EmptyEvent())
    }
  }

  // MARK: - Search

  private func search(_ endpoint: Search) async throws -> Response {
    switch endpoint {
    case let .search(query, type):
      if type == "statuses", let id = WarpnetFormat.statusId(fromURL: query) {
        let status = try await repository.status(at: repository.address(of: id))
        return try response(MastodonSearchResults(accounts: [], statuses: [status]))
      }
      guard type == nil || type == "accounts" else { throw WarpnetRoutingError.unsupported("search \(type ?? "")") }
      return try await response(MastodonSearchResults(accounts: searchAccounts(query), statuses: []))
    case let .accountsSearch(query):
      return try await response(searchAccounts(query))
    }
  }

  private func searchAccounts(_ query: String) async throws -> [MastodonAccount] {
    let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else { return [] }
    var accounts: [MastodonAccount] = []
    if let handle = Self.fediverseHandle(trimmed),
       let gateway: GatewaySettings = try? await repository.call(ProtocolIds.PRIVATE_GET_SETTINGS_GATEWAY, EmptyEvent()),
       !gateway.nodeId.isEmpty,
       let user = try? await repository.user(handle, nodeHint: gateway.nodeId),
       !user.username.isEmpty
    {
      accounts.append(WarpnetMapper.account(user))
    }
    let page: UsersResponse = try await repository.call(ProtocolIds.PUBLIC_GET_USERS_SEARCH,
                                                        SearchUsersEvent(query: trimmed, limit: WarpnetRepository.pageSize))
    accounts.append(contentsOf: page.users.map { WarpnetMapper.account($0) })
    return accounts
  }

  static func fediverseHandle(_ query: String) -> String? {
    if let match = query.firstMatch(of: #/^@?([\w.-]+)@([a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+)$/#) {
      return "\(match.1)@\(match.2.lowercased())"
    }
    if let match = query.firstMatch(of: #/^https?://(?:www\.)?([a-zA-Z0-9-]+(?:\.[a-zA-Z0-9-]+)+)/@([\w.-]+)/?$/#) {
      return "\(match.2)@\(match.1.lowercased())"
    }
    return nil
  }

  // MARK: - Polls

  private func polls(_ endpoint: Polls) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case let .poll(id):
      let address = try await repository.address(of: id).original
      return try await response(pollStatus(address).poll)
    case let .vote(id, votes):
      let address = try await repository.address(of: id).original
      let tweet = try await repository.tweet(address)
      guard let option = votes.first, let optionsNum = tweet.poll?.options.count else {
        throw WarpnetRoutingError.unsupported(endpoint.path())
      }
      try await repository.send(ProtocolIds.PUBLIC_POST_POLL_VOTE,
                                PollVoteEvent(tweetId: address.tweetId,
                                              userId: address.userId,
                                              ownerId: pairing.userId,
                                              option: option,
                                              optionsNum: optionsNum))
      return try await response(pollStatus(address).poll)
    }
  }

  private func pollStatus(_ address: StatusAddress) async throws -> MastodonStatus {
    let status = try await repository.status(at: address)
    guard status.poll != nil else { throw WarpnetRoutingError.unknownStatus(address.id) }
    return status
  }

  // MARK: - Media

  private func mediaAttachment(id: String, ownerId: String) -> MastodonMediaAttachment {
    let isVideo = id.hasPrefix(WarpnetRepository.videoPrefix)
    let key = String(id.dropFirst(isVideo ? WarpnetRepository.videoPrefix.count : WarpnetRepository.imagePrefix.count))
    let url = (isVideo ? WarpnetFormat.videoURL(userId: ownerId, key: key) : WarpnetFormat.imageURL(userId: ownerId, key: key)) ?? ""
    return MastodonMediaAttachment(id: id, type: isVideo ? "video" : "image", url: url, previewUrl: isVideo ? nil : url, description: nil)
  }

  // MARK: - Follow requests, markers, instance

  private func followRequests(_ endpoint: FollowRequests) async throws -> Response {
    let pairing = try await repository.viewer()
    switch endpoint {
    case .list:
      let page: GetFollowRequestsResponse = try await repository.call(ProtocolIds.PRIVATE_GET_FOLLOW_REQUESTS,
                                                                      GetFollowRequestsEvent(userId: pairing.userId,
                                                                                             limit: WarpnetRepository.pageSize))
      return try await response(repository.accounts(page.followerIds))
    case let .accept(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_FOLLOW_REQUEST_AUTHORIZE,
                                FollowRequestActionEvent(userId: pairing.userId, followerId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId))
    case let .reject(id):
      try await repository.send(ProtocolIds.PRIVATE_POST_FOLLOW_REQUEST_REJECT,
                                FollowRequestActionEvent(userId: pairing.userId, followerId: id))
      return try await response(relationship(with: id, viewerId: pairing.userId))
    }
  }

  private func markers(_ endpoint: Markers) async throws -> Response {
    switch endpoint {
    case let .markNotifications(lastReadId):
      try await repository.send(ProtocolIds.PRIVATE_POST_NOTIFICATIONS_READ, EmptyEvent())
      return try response(MastodonMarker(notifications: .init(lastReadId: lastReadId,
                                                              version: 1,
                                                              updatedAt: WarpnetFormat.serverDate(Date()))))
    }
  }

  private func instances(_ endpoint: Instances) async throws -> Response {
    switch endpoint {
    case .instance:
      return try response(WarpnetMapper.instance())
    }
  }

  // MARK: - Paging

  private func pagedStatuses(scope: String,
                             maxId: String?,
                             newerThan: String?,
                             filterContext: String? = nil,
                             keep: @escaping @Sendable (WarpnetTweet) -> Bool = { _ in true },
                             fetch: @escaping @Sendable (String) async throws -> WarpnetRepository.Page) async throws -> Response
  {
    if let newerThan {
      var tweets: [WarpnetTweet] = []
      var seen = Set<String>()
      var cursor: String? = ""
      for _ in 0 ..< Self.maxResumePages {
        guard let current = cursor else { break }
        let page = try await fetch(current)
        cursor = Self.isEnd(page.cursor) ? nil : page.cursor
        var kept = page.tweets.filter(keep)
        let anchor = kept.firstIndex { WarpnetMapper.address($0).id == newerThan }
        if let anchor {
          kept = Array(kept[..<anchor])
        }
        tweets += kept.filter { seen.insert(WarpnetMapper.address($0).id).inserted }
        if anchor != nil { break }
      }
      return try await response(repository.statuses(tweets, filterContext: filterContext))
    }

    var tweets: [WarpnetTweet] = []
    var next: String? = ""
    if let maxId {
      if let known = await repository.cursor(scope: scope, after: maxId) {
        next = Self.isEnd(known) ? nil : known
      } else if let resumed = try await resume(after: maxId, keep: keep, fetch: fetch) {
        tweets = resumed.tweets
        next = resumed.next
      } else {
        return try response([MastodonStatus]())
      }
    }

    var fetched = 0
    while tweets.count < Self.minimumPage, let cursor = next, fetched < Self.maxPagesPerRequest {
      let page = try await fetch(cursor)
      tweets += page.tweets.filter(keep)
      next = Self.isEnd(page.cursor) ? nil : page.cursor
      fetched += 1
    }

    let statuses = await repository.statuses(tweets, filterContext: filterContext)
    if let last = statuses.last {
      await repository.storeCursor(next ?? WarpnetRepository.endCursor, scope: scope, after: last.id)
    }
    return try response(statuses)
  }

  private func resume(after maxId: String,
                      keep: @Sendable (WarpnetTweet) -> Bool,
                      fetch: @Sendable (String) async throws -> WarpnetRepository.Page) async throws
    -> (tweets: [WarpnetTweet], next: String?)?
  {
    var cursor: String? = ""
    for _ in 0 ..< Self.maxResumePages {
      guard let current = cursor else { return nil }
      let page = try await fetch(current)
      let kept = page.tweets.filter(keep)
      cursor = Self.isEnd(page.cursor) ? nil : page.cursor
      if let index = kept.firstIndex(where: { WarpnetMapper.address($0).id == maxId }) {
        return (Array(kept[(index + 1)...]), cursor)
      }
    }
    return nil
  }

  static func isEnd(_ cursor: String) -> Bool {
    cursor.isEmpty || cursor == WarpnetRepository.endCursor
  }

  private func nextToken(_ cursor: String, count: Int) -> String? {
    count == 0 || Self.isEnd(cursor) ? nil : cursor
  }

  private func response(_ value: some Encodable, next: String? = nil) throws -> Response {
    try Response(body: encode(value), next: next)
  }

  private func encode(_ value: some Encodable) throws -> Data {
    try MastodonJSON.encode(value)
  }
}

struct EmptyEvent: Encodable, Sendable {}

struct MastodonMarker: Encodable, Sendable {
  struct Content: Encodable, Sendable {
    let lastReadId: String
    let version: Int
    let updatedAt: String
  }

  let notifications: Content?
}
