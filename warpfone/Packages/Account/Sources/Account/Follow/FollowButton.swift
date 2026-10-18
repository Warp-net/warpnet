import ButtonKit
import Combine
import Foundation
import Models
import Network
import Observation
import SwiftUI
import OSLog

@MainActor
@Observable public class FollowButtonViewModel {
  var client: Client?

  public let accountId: String
  public let relationshipUpdated: (Relationship) -> Void
  public private(set) var relationship: Relationship

  public init(accountId: String,
              relationship: Relationship,
              relationshipUpdated: @escaping ((Relationship) -> Void))
  {
    self.accountId = accountId
    self.relationship = relationship
    self.relationshipUpdated = relationshipUpdated
  }

  func follow() async throws {
    guard let client else { return }
    do {
      relationship = try await client.post(endpoint: Accounts.follow(id: accountId))
      relationshipUpdated(relationship)
    } catch {
      throw error
    }
  }

  func unfollow() async throws {
    guard let client else { return }
    do {
      relationship = try await client.post(endpoint: Accounts.unfollow(id: accountId))
      relationshipUpdated(relationship)
    } catch {
      throw error
    }
  }
}

public struct FollowButton: View {
  @Environment(Client.self) private var client
  @State private var viewModel: FollowButtonViewModel

  public init(viewModel: FollowButtonViewModel) {
    _viewModel = .init(initialValue: viewModel)
  }

  public var body: some View {
    VStack(alignment: .trailing) {
      AsyncButton {
        if viewModel.relationship.following {
          try await viewModel.unfollow()
        } else {
          try await viewModel.follow()
        }
      } label: {
        if viewModel.relationship.requested == true {
          Text("account.follow.requested")
        } else {
          Text(viewModel.relationship.following ? "account.follow.following" : "account.follow.follow")
            .accessibilityLabel("account.follow.following")
            .accessibilityValue(viewModel.relationship.following ? "accessibility.general.toggle.on" : "accessibility.general.toggle.off")
        }
      }
    }
    .buttonStyle(.bordered)
    .onAppear {
      viewModel.client = client
    }
  }
}
