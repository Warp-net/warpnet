import Foundation

public enum Instances: Endpoint {
  case instance

  public func path() -> String {
    "instance"
  }

  public func queryItems() -> [URLQueryItem]? {
    nil
  }
}
