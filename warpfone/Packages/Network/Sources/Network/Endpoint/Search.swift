import Foundation

public enum Search: Endpoint {
  case search(query: String, type: String?)
  case accountsSearch(query: String)

  public func path() -> String {
    switch self {
    case .search:
      "search"
    case .accountsSearch:
      "accounts/search"
    }
  }

  public func queryItems() -> [URLQueryItem]? {
    switch self {
    case let .search(query, type):
      var params: [URLQueryItem] = [.init(name: "q", value: query)]
      if let type {
        params.append(.init(name: "type", value: type))
      }
      return params
    case let .accountsSearch(query):
      return [.init(name: "q", value: query)]
    }
  }
}
