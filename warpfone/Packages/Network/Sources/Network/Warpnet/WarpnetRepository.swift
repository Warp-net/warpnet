import Foundation
import WarpnetTransport

public enum WarpnetRoutingError: Error, Equatable, LocalizedError {
  case unsupported(String)
  case unknownStatus(String)
  case emptyResponse(String)
  case invalidMedia(String)

  public var errorDescription: String? {
    switch self {
    case let .unsupported(path):
      "Warpnet does not support \(path)"
    case let .unknownStatus(id):
      "Unknown post \(id)"
    case let .emptyResponse(route):
      "The node returned nothing for \(route)"
    case let .invalidMedia(reason):
      reason
    }
  }
}

public extension Error {
  var isMissingTweet: Bool {
    switch self as Error {
    case WarpnetRoutingError.unknownStatus:
      true
    case let WarpnetError.protocolError(_, message):
      message.hasSuffix("tweet not found") || message.hasSuffix("tweet is moderated")
    default:
      false
    }
  }
}

actor WarpnetRepository {
  struct Page: Sendable {
    let tweets: [WarpnetTweet]
    let cursor: String
  }

  static let pageSize = 40
  static let endCursor = "end"
  static let imagePrefix = "img:"
  static let videoPrefix = "vid:"

  private static let tweetCacheMinAge: TimeInterval = 3600
  private static let bookmarksKey = "warpnet.bookmarks"
  private static let maxConcurrentRequests = 8
  private static let filtersTTL: TimeInterval = 300

  private let session: WarpnetSession
  private let defaults: UserDefaults
  private var users = LRUCache<String, WarpnetUser>(maxCost: 256, ttl: 300)
  private var tweets = LRUCache<String, WarpnetTweet>(maxCost: 512, ttl: 6 * 3600)
  private var addresses = LRUCache<String, StatusAddress>(maxCost: 4096)
  private var stats = LRUCache<String, TweetStatsResponse>(maxCost: 2048, ttl: 300)
  private var images = LRUCache<String, Data>(maxCost: 8 << 20)
  private var cursors = LRUCache<String, String>(maxCost: 2048)
  private var retweeted = Set<String>()
  private var bookmarks: Set<String>
  private var filterList: [WarpnetFilter]?
  private var filtersFetchedAt = Date.distantPast

  init(session: WarpnetSession, defaults: UserDefaults = .standard) {
    self.session = session
    self.defaults = defaults
    bookmarks = Set(defaults.stringArray(forKey: Self.bookmarksKey) ?? [])
  }

  // MARK: - Transport

  func viewer() async throws -> AuthNodeInfo {
    guard let pairing = await session.pairing else { throw WarpnetError.notConnected }
    return pairing
  }

  func call<Response: Decodable>(_ route: String, _ body: some Encodable) async throws -> Response {
    let raw = try await session.client.request(route, body: WarpnetJSON.encode(body))
    guard !raw.isEmpty else { throw WarpnetRoutingError.emptyResponse(route) }
    return try WarpnetJSON.decode(Response.self, from: raw)
  }

  func send(_ route: String, _ body: some Encodable) async throws {
    _ = try await session.client.request(route, body: WarpnetJSON.encode(body))
  }

  // MARK: - Users

  func user(_ id: String, nodeHint: String = "") async throws -> WarpnetUser {
    if let cached = users.value(for: id) {
      return cached
    }
    let pairing = try await viewer()
    let user: WarpnetUser = try await call(ProtocolIds.PUBLIC_GET_USER,
                                           GetUserEvent(userId: id, nodeId: nodeHint.isEmpty ? pairing.nodeId : nodeHint))
    guard !user.id.isEmpty else { throw WarpnetRoutingError.emptyResponse(ProtocolIds.PUBLIC_GET_USER) }
    users.set(user, for: id)
    return user
  }

  func forgetUser(_ id: String) {
    users.remove(id)
  }

  func users(_ ids: [String]) async -> [String: WarpnetUser] {
    let unique = Array(Set(ids.filter { !$0.isEmpty }))
    let found = await concurrentMap(unique) { id in try? await self.user(id) }
    return Dictionary(found.compactMap { $0 }.map { ($0.id, $0) }, uniquingKeysWith: { first, _ in first })
  }

  func accounts(_ ids: [String]) async -> [MastodonAccount] {
    let resolved = await users(ids)
    return ids.compactMap { resolved[$0].map { WarpnetMapper.account($0) } }
  }

  // MARK: - Tweets

  func tweet(_ address: StatusAddress) async throws -> WarpnetTweet {
    if let cached = tweets.value(for: address.tweetId) {
      return cached
    }
    let known = addresses.value(for: address.tweetId)
    let parentId = address.parentId.isEmpty ? known?.parentId ?? "" : address.parentId
    let rootId = address.rootId.isEmpty ? known?.rootId ?? "" : address.rootId
    let tweet: WarpnetTweet = try await call(ProtocolIds.PUBLIC_GET_TWEET,
                                             GetTweetEvent(tweetId: address.tweetId,
                                                           userId: address.userId,
                                                           parentId: parentId,
                                                           rootId: rootId.isEmpty ? parentId : rootId))
    guard !tweet.id.isEmpty else { throw WarpnetRoutingError.unknownStatus(address.id) }
    remember(tweet)
    return tweet
  }

  func address(of id: String) throws -> StatusAddress {
    guard var address = StatusAddress(id: id) else { throw WarpnetRoutingError.unknownStatus(id) }
    if address.parentId.isEmpty, let known = addresses.value(for: address.tweetId) {
      address.parentId = known.parentId
    }
    return address
  }

  func remember(_ tweet: WarpnetTweet) {
    let address = WarpnetMapper.address(tweet)
    if !address.isRetweet || addresses.value(for: address.tweetId) == nil {
      addresses.set(address.original, for: address.tweetId)
    }
    guard !address.isRetweet else { return }
    let lastModified = WarpnetFormat.date(fromRFC3339: tweet.updatedAt) ?? WarpnetFormat.date(fromRFC3339: tweet.createdAt)
    if let lastModified, Date().timeIntervalSince(lastModified) >= Self.tweetCacheMinAge {
      tweets.set(tweet, for: address.tweetId)
    }
  }

  func forgetTweet(_ tweetId: String) {
    tweets.remove(tweetId)
  }

  func homePage(cursor: String) async throws -> Page {
    let pairing = try await viewer()
    return try await page(ProtocolIds.PRIVATE_GET_TIMELINE, GetAllTweetsEvent(userId: pairing.userId, cursor: cursor, limit: Self.pageSize))
  }

  func userPage(userId: String, cursor: String) async throws -> Page {
    try await page(ProtocolIds.PUBLIC_GET_TWEETS, GetAllTweetsEvent(userId: userId, cursor: cursor, limit: Self.pageSize))
  }

  func repliesPage(of address: StatusAddress, cursor: String = "") async throws -> Page {
    try await page(ProtocolIds.PUBLIC_GET_TWEETS,
                   GetAllTweetsEvent(rootId: address.threadRootId,
                                     parentId: address.tweetId,
                                     rootUserId: address.userId,
                                     cursor: cursor,
                                     limit: Self.pageSize))
  }

  private func page(_ route: String, _ event: GetAllTweetsEvent) async throws -> Page {
    let response: TweetsResponse = try await call(route, event)
    response.tweets.forEach { remember($0) }
    return Page(tweets: response.tweets, cursor: response.cursor)
  }

  func ancestors(of tweet: WarpnetTweet, maxDepth: Int = 32) async -> [WarpnetTweet] {
    var chain: [WarpnetTweet] = []
    var current = tweet
    while let parent = WarpnetMapper.parentAddress(current), chain.count < maxDepth {
      let known = addresses.value(for: parent.tweetId)
      let lookup = StatusAddress(tweetId: parent.tweetId,
                                 userId: known?.userId ?? parent.userId,
                                 rootId: current.rootId,
                                 parentId: known?.parentId ?? "")
      guard let next = try? await self.tweet(lookup) else { break }
      chain.append(next)
      current = next
    }
    return chain.reversed()
  }

  func descendants(of address: StatusAddress, depth: Int = 3) async -> [WarpnetTweet] {
    guard depth > 0, let page = try? await repliesPage(of: address) else { return [] }
    let replies = page.tweets.sorted { ($0.createdAt ?? "") < ($1.createdAt ?? "") }
    let pairing = try? await viewer()
    let replyStats = await statsFor(replies.map { WarpnetMapper.address($0).tweetId }, viewerId: pairing?.userId ?? "")
    var thread: [WarpnetTweet] = []
    for reply in replies {
      thread.append(reply)
      let replyAddress = WarpnetMapper.address(reply).original
      if (replyStats[replyAddress.tweetId]?.repliesCount ?? 0) > 0 {
        thread.append(contentsOf: await descendants(of: replyAddress, depth: depth - 1))
      }
    }
    return thread
  }

  // MARK: - Hydration

  func statuses(_ tweets: [WarpnetTweet], filterContext: String? = nil) async -> [MastodonStatus] {
    guard !tweets.isEmpty, let pairing = try? await viewer() else { return [] }
    tweets.forEach { remember($0) }
    let originalIds = tweets.map { WarpnetMapper.address($0).tweetId }
    let authorIds = tweets.map(\.userId) + tweets.compactMap(\.retweetedBy)
    async let resolvedUsers = users(authorIds)
    async let resolvedStats = statsFor(originalIds, viewerId: pairing.userId)
    async let resolvedPolls = pollsFor(tweets, voterId: pairing.userId)
    var filters: [WarpnetFilter] = []
    if let filterContext {
      filters = await activeFilters(context: filterContext, viewerId: pairing.userId)
    }
    let (users, stats, polls) = await (resolvedUsers, resolvedStats, resolvedPolls)
    return tweets.map { build($0, users: users, stats: stats, polls: polls, filters: filters, viewerId: pairing.userId) }
  }

  func status(_ tweet: WarpnetTweet, filterContext: String? = nil) async -> MastodonStatus? {
    await statuses([tweet], filterContext: filterContext).first
  }

  func status(at address: StatusAddress, filterContext: String? = nil) async throws -> MastodonStatus {
    let tweet = try await tweet(address.original)
    guard let status = await status(tweet, filterContext: filterContext) else { throw WarpnetRoutingError.unknownStatus(address.id) }
    return status
  }

  private func build(_ tweet: WarpnetTweet,
                     users: [String: WarpnetUser],
                     stats: [String: TweetStatsResponse],
                     polls: [String: PollResultsResponse],
                     filters: [WarpnetFilter],
                     viewerId: String) -> MastodonStatus
  {
    let address = WarpnetMapper.address(tweet)
    let id = address.tweetId
    let state = ViewerState(stats: stats[id] ?? self.stats.value(for: id),
                            poll: polls[id],
                            isBookmarked: bookmarks.contains(id),
                            isRetweeted: retweeted.contains(id) || (address.isRetweet && address.retweetedBy == viewerId))
    let status = WarpnetMapper.status(tweet, author: users[tweet.userId], viewer: state, quote: quoteURL(tweet))
    if !filters.isEmpty {
      status.filtered = WarpnetMapper.filterResults(text: tweet.text, filters: filters)
    }
    guard address.isRetweet else { return status }
    let retweeter = users[address.retweetedBy].map { WarpnetMapper.account($0) }
      ?? WarpnetMapper.account(userId: address.retweetedBy, username: "")
    return WarpnetMapper.retweet(status, address: address, retweeter: retweeter)
  }

  private func quoteURL(_ tweet: WarpnetTweet) -> String? {
    guard let quotedTweetId = tweet.quotedTweetId, !quotedTweetId.isEmpty,
          let quotedUserId = tweet.quotedUserId, !quotedUserId.isEmpty
    else { return nil }
    return WarpnetFormat.statusURL(id: StatusAddress(tweetId: quotedTweetId, userId: quotedUserId).id)
  }

  func statsFor(_ tweetIds: [String], viewerId: String) async -> [String: TweetStatsResponse] {
    guard !viewerId.isEmpty else { return [:] }
    let unique = Array(Set(tweetIds))
    let found = await concurrentMap(unique) { id -> (String, TweetStatsResponse)? in
      guard let response: TweetStatsResponse = try? await self.call(ProtocolIds.PUBLIC_GET_TWEET_STATS,
                                                                     GetTweetStatsEvent(tweetId: id, userId: viewerId))
      else { return nil }
      return (id, response)
    }
    var result: [String: TweetStatsResponse] = [:]
    for case let (id, response)? in found {
      result[id] = response
      stats.set(response, for: id)
    }
    return result
  }

  private func pollsFor(_ tweets: [WarpnetTweet], voterId: String) async -> [String: PollResultsResponse] {
    let polled = tweets.filter { !($0.poll?.options.isEmpty ?? true) }
    let found = await concurrentMap(polled) { tweet -> (String, PollResultsResponse)? in
      let address = WarpnetMapper.address(tweet)
      guard let results = try? await self.pollResults(tweetId: address.tweetId,
                                                      authorId: address.userId,
                                                      voterId: voterId,
                                                      optionsNum: tweet.poll?.options.count ?? 0)
      else { return nil }
      return (address.tweetId, results)
    }
    return Dictionary(found.compactMap { $0 }, uniquingKeysWith: { first, _ in first })
  }

  // MARK: - Viewer state

  func activeFilters(context: String, viewerId: String, now: Date = Date()) async -> [WarpnetFilter] {
    if filterList == nil || now.timeIntervalSince(filtersFetchedAt) > Self.filtersTTL {
      let page: GetFiltersResponse? = try? await call(ProtocolIds.PRIVATE_GET_SETTINGS_FILTERS,
                                                      GetFiltersEvent(userId: viewerId, limit: 100))
      filterList = page?.filters ?? filterList ?? []
      filtersFetchedAt = now
    }
    return (filterList ?? []).filter { filter in
      filter.context.contains(context) && (WarpnetFormat.date(fromRFC3339: filter.expiresAt).map { $0 > now } ?? true)
    }
  }

  func setFilters(_ filters: [WarpnetFilter]) {
    filterList = filters
    filtersFetchedAt = Date()
  }

  func forgetFilters() {
    filterList = nil
  }

  func cachedStats(_ tweetId: String) -> TweetStatsResponse? {
    stats.value(for: tweetId)
  }

  func setStats(_ value: TweetStatsResponse, for tweetId: String) {
    stats.set(value, for: tweetId)
  }

  func setRetweeted(_ isRetweeted: Bool, tweetId: String) {
    if isRetweeted {
      retweeted.insert(tweetId)
    } else {
      retweeted.remove(tweetId)
    }
  }

  func setBookmarked(_ isBookmarked: Bool, tweetId: String) {
    if isBookmarked {
      bookmarks.insert(tweetId)
    } else {
      bookmarks.remove(tweetId)
    }
    defaults.set(Array(bookmarks), forKey: Self.bookmarksKey)
  }

  // MARK: - Pagination

  func storeCursor(_ cursor: String, scope: String, after lastId: String) {
    cursors.set(cursor.isEmpty ? Self.endCursor : cursor, for: "\(scope)|\(lastId)")
  }

  func cursor(scope: String, after lastId: String) -> String? {
    cursors.value(for: "\(scope)|\(lastId)")
  }

  // MARK: - Polls

  func pollResults(tweetId: String, authorId: String, voterId: String, optionsNum: Int) async throws -> PollResultsResponse {
    try await call(ProtocolIds.PUBLIC_GET_POLL,
                   GetPollEvent(tweetId: tweetId, userId: authorId, ownerId: voterId, optionsNum: optionsNum))
  }

  // MARK: - Media

  func imageData(userId: String, key: String) async throws -> Data {
    let cacheKey = "\(userId)/\(key)"
    if let cached = images.value(for: cacheKey) {
      return cached
    }
    let response: GetImageResponse = try await call(ProtocolIds.PUBLIC_GET_IMAGE, GetImageEvent(userId: userId, key: key))
    guard let data = Self.decodeFile(response.file) else {
      throw WarpnetRoutingError.emptyResponse(ProtocolIds.PUBLIC_GET_IMAGE)
    }
    images.set(data, for: cacheKey, cost: data.count)
    return data
  }

  func videoData(userId: String, key: String) async throws -> Data {
    let response: GetVideoResponse = try await call(ProtocolIds.PUBLIC_GET_VIDEO, GetVideoEvent(userId: userId, key: key))
    guard let data = Self.decodeFile(response.file) else {
      throw WarpnetRoutingError.emptyResponse(ProtocolIds.PUBLIC_GET_VIDEO)
    }
    return data
  }

  func uploadImage(_ data: Data, mimeType: String) async throws -> String {
    let response: UploadImageResponse = try await call(ProtocolIds.PRIVATE_POST_UPLOAD_IMAGE,
                                                       UploadImageEvent(image1: "\(mimeType),\(data.base64EncodedString())"))
    guard let key = response.keys.first else { throw WarpnetRoutingError.emptyResponse(ProtocolIds.PRIVATE_POST_UPLOAD_IMAGE) }
    return key
  }

  func uploadVideo(_ data: Data, mimeType: String) async throws -> String {
    let response: UploadVideoResponse = try await call(ProtocolIds.PRIVATE_POST_UPLOAD_VIDEO,
                                                       UploadVideoEvent(video: "data:\(mimeType);base64,\(data.base64EncodedString())"))
    guard !response.key.isEmpty else { throw WarpnetRoutingError.emptyResponse(ProtocolIds.PRIVATE_POST_UPLOAD_VIDEO) }
    return response.key
  }

  static func decodeFile(_ file: String) -> Data? {
    guard !file.isEmpty else { return nil }
    let base64 = file.firstIndex(of: ",").map { String(file[file.index(after: $0)...]) } ?? file
    guard let data = Data(base64Encoded: base64, options: .ignoreUnknownCharacters), !data.isEmpty else { return nil }
    return data
  }

  // MARK: - Helpers

  func concurrentMap<Input: Sendable, Output: Sendable>(_ inputs: [Input],
                                                                _ transform: @escaping @Sendable (Input) async -> Output) async -> [Output]
  {
    guard !inputs.isEmpty else { return [] }
    return await withTaskGroup(of: (Int, Output).self) { group in
      var results = [Output?](repeating: nil, count: inputs.count)
      var next = 0
      for _ in 0 ..< min(Self.maxConcurrentRequests, inputs.count) {
        let index = next
        group.addTask { (index, await transform(inputs[index])) }
        next += 1
      }
      while let (index, output) = await group.next() {
        results[index] = output
        if next < inputs.count {
          let index = next
          group.addTask { (index, await transform(inputs[index])) }
          next += 1
        }
      }
      return results.compactMap { $0 }
    }
  }
}
