import Foundation

public enum Notifications: Endpoint {
  case notifications(minId: String?,
                     maxId: String?,
                     types: [String]?,
                     limit: Int)

  public func path() -> String {
    "notifications"
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .notifications(mindId, maxId, types, limit):
      var params = makePaginationParam(sinceId: nil, maxId: maxId, mindId: mindId) ?? []
      params.append(.init(name: "limit", value: String(limit)))
      if let types {
        for type in types {
          params.append(.init(name: "exclude_types[]", value: type))
        }
      }
      return params
    }
  }
}
