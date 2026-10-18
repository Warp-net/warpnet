import Foundation

public enum Media: Endpoint {
  case medias

  public func path() -> String {
    "media"
  }

  public func queryItems() -> [URLQueryItem]? {
    nil
  }
}
