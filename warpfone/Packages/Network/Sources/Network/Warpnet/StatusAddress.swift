import Foundation

public struct StatusAddress: Hashable, Sendable {
  public static let retweetPrefix = "RT:"

  public let tweetId: String
  public let userId: String
  public let rootId: String
  public let retweetedBy: String
  public var parentId: String

  public init(tweetId: String, userId: String, rootId: String = "", retweetedBy: String = "", parentId: String = "") {
    let id = tweetId.hasPrefix(Self.retweetPrefix) ? String(tweetId.dropFirst(Self.retweetPrefix.count)) : tweetId
    self.tweetId = id
    self.userId = userId
    self.rootId = rootId == id ? "" : rootId
    self.retweetedBy = retweetedBy
    self.parentId = parentId == id ? "" : parentId
  }

  public var isRetweet: Bool {
    !retweetedBy.isEmpty
  }

  public var original: StatusAddress {
    StatusAddress(tweetId: tweetId, userId: userId, rootId: rootId, parentId: parentId)
  }

  public var threadRootId: String {
    rootId.isEmpty ? tweetId : rootId
  }

  public var partitionId: String {
    parentId.isEmpty ? rootId : parentId
  }

  public var id: String {
    var parts = [tweetId, userId, rootId, retweetedBy].map(Self.escape)
    while parts.count > 2, parts.last?.isEmpty == true {
      parts.removeLast()
    }
    return parts.joined(separator: "|")
  }

  public init?(id: String) {
    let parts = id.split(separator: "|", omittingEmptySubsequences: false).map { Self.unescape(String($0)) }
    guard parts.count >= 2, !parts[0].isEmpty, !parts[1].isEmpty else { return nil }
    self.init(tweetId: parts[0],
              userId: parts[1],
              rootId: parts.count > 2 ? parts[2] : "",
              retweetedBy: parts.count > 3 ? parts[3] : "")
  }

  private static func escape(_ part: String) -> String {
    part.replacingOccurrences(of: "%", with: "%25").replacingOccurrences(of: "|", with: "%7C")
  }

  private static func unescape(_ part: String) -> String {
    part.replacingOccurrences(of: "%7C", with: "|").replacingOccurrences(of: "%25", with: "%")
  }
}
