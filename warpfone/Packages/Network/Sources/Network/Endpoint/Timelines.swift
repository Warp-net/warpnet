import Foundation

public enum Timelines: Endpoint {
  case home(sinceId: String?, maxId: String?, minId: String?)

  public func path() -> String {
    "timelines/home"
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .home(sinceId, maxId, mindId):
      makePaginationParam(sinceId: sinceId, maxId: maxId, mindId: mindId)
    }
  }
}
