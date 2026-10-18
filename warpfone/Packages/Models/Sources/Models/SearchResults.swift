import Foundation

public struct SearchResults: Decodable {
  enum CodingKeys: String, CodingKey {
    case accounts, statuses
  }

  public let accounts: [Account]
  public var relationships: [Relationship] = []
  public let statuses: [Status]

  public var isEmpty: Bool {
    accounts.isEmpty && statuses.isEmpty
  }
}

extension SearchResults: Sendable {}
