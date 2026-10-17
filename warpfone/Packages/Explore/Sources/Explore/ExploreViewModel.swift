// Modified by Warpnet. See NOTICE.

import Models
import Network
import Observation
import SwiftUI

@MainActor
@Observable class ExploreViewModel {
  enum SearchScope: String, CaseIterable {
    case all, people

    var localizedString: LocalizedStringKey {
      switch self {
      case .all:
        .init("explore.scope.all")
      case .people:
        .init("explore.scope.people")
      }
    }
  }

  var client: Client? {
    didSet {
      if oldValue != client {
        isLoaded = false
        results = [:]
        suggestedAccounts = []
      }
    }
  }

  var allSectionsEmpty: Bool {
    suggestedAccounts.isEmpty
  }

  var searchQuery = "" {
    didSet {
      isSearching = true
    }
  }

  var results: [String: SearchResults] = [:]
  var isLoaded = false
  var isSearching = false
  var suggestedAccounts: [Account] = []
  var suggestedAccountsRelationShips: [Relationship] = []
  var searchScope: SearchScope = .all
  var scrollToTopVisible: Bool = false
  var isSearchPresented: Bool = false

  init() {}

  func fetchTrending() async {
    guard let client else { return }
    do {
      suggestedAccounts = try await client.get(endpoint: Accounts.suggestions)
      suggestedAccountsRelationShips = try await client.get(endpoint: Accounts.relationships(ids: suggestedAccounts.map(\.id)))
      withAnimation {
        isLoaded = true
      }
    } catch {
      isLoaded = true
    }
  }

  func search() async {
    guard let client, !searchQuery.isEmpty else { return }
    do {
      try await Task.sleep(for: .milliseconds(250))
      var results: SearchResults = try await client.get(endpoint: Search.search(query: searchQuery,
                                                                                type: nil,
                                                                                offset: nil,
                                                                                following: nil),
                                                        forceVersion: .v2)
      let relationships: [Relationship] =
        try await client.get(endpoint: Accounts.relationships(ids: results.accounts.map(\.id)))
      results.relationships = relationships
      withAnimation {
        self.results[searchQuery] = results
        isSearching = false
      }
    } catch {
      isSearching = false
    }
  }
}
