import Foundation

public enum TweetViews: Endpoint {
  case record(statusId: String)

  public func path() -> String {
    switch self {
    case let .record(statusId):
      "warpnet/views/\(statusId)"
    }
  }

  public func queryItems() -> [URLQueryItem]? {
    nil
  }
}
