import Foundation

public enum Markers: Endpoint {
  case markNotifications(lastReadId: String)

  public func path() -> String {
    "markers"
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .markNotifications(lastReadId):
      [URLQueryItem(name: "notifications[last_read_id]", value: lastReadId)]
    }
  }
}
