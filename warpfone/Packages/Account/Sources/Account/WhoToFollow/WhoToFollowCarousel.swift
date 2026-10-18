import DesignSystem
import Env
import Models
import Network
import SwiftUI

/// The "Who to follow" row warpdroid puts into the home timeline: the node's
/// recommendations, which already leave out you, offline nodes and people you follow.
@MainActor
public struct WhoToFollowCarousel: View {
  @Environment(Client.self) private var client
  @Environment(RouterPath.self) private var routerPath
  @Environment(Theme.self) private var theme

  @State private var accounts: [Account] = []
  @State private var relationships: [Relationship] = []

  public init() {}

  public var body: some View {
    // A VStack, not a Group: an empty Group has no view to run `.task` on.
    VStack(spacing: 0) {
      if accounts.isEmpty {
        Color.clear.frame(height: 1)
      } else {
        VStack(alignment: .leading, spacing: 8) {
          Text("warpnet.who-to-follow")
            .font(.scaledHeadline)
            .padding(.horizontal, .layoutPadding)
          ScrollView(.horizontal, showsIndicators: false) {
            LazyHStack(spacing: 12) {
              ForEach(accounts) { account in
                card(account)
              }
            }
            .padding(.horizontal, .layoutPadding)
          }
        }
        .padding(.vertical, 8)
      }
    }
    .task {
      await fetch()
    }
  }

  private func card(_ account: Account) -> some View {
    VStack(spacing: 6) {
      AvatarView(account.avatar, config: .account)
      Text(account.safeDisplayName)
        .font(.scaledSubheadline)
        .fontWeight(.semibold)
        .lineLimit(1)
      Text("@\(account.acct)")
        .font(.scaledFootnote)
        .foregroundStyle(.secondary)
        .lineLimit(1)
      if let relationship = relationships.first(where: { $0.id == account.id }) {
        FollowButton(viewModel: .init(accountId: account.id,
                                      relationship: relationship,
                                      relationshipUpdated: { relationship in
                                        relationships.removeAll { $0.id == relationship.id }
                                        relationships.append(relationship)
                                      }))
      }
    }
    .frame(width: 150)
    .padding(12)
    .background(theme.secondaryBackgroundColor)
    .clipShape(RoundedRectangle(cornerRadius: 12))
    .contentShape(Rectangle())
    .onTapGesture {
      routerPath.navigate(to: .accountDetailWithAccount(account: account))
    }
  }

  private func fetch() async {
    guard client.isAuth, accounts.isEmpty else { return }
    guard let suggested: [Account] = try? await client.get(endpoint: Accounts.suggestions),
          !suggested.isEmpty
    else { return }
    let found: [Relationship] = (try? await client.get(endpoint: Accounts.relationships(ids: suggested.map(\.id)))) ?? []
    withAnimation {
      relationships = found
      accounts = suggested
    }
  }
}
