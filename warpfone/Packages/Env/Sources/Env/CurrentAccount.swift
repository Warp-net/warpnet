// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Network
import Observation

@MainActor
@Observable public class CurrentAccount {
  private static var accountsCache: [String: Account] = [:]

  public private(set) var account: Account?
  public private(set) var followRequests: [Account] = []
  public private(set) var isUpdating: Bool = false
  public private(set) var updatingFollowRequestAccountIds = Set<String>()
  public private(set) var isLoadingAccount: Bool = false

  private var client: Client?

  public static let shared = CurrentAccount()

  private init() {}

  public func setClient(client: Client) {
    self.client = client
    guard client.isAuth else { return }
    Task(priority: .userInitiated) {
      await fetchUserData()
    }
  }

  private func fetchUserData() async {
    await withTaskGroup(of: Void.self) { group in
      group.addTask { await self.fetchCurrentAccount() }
      group.addTask { await self.fetchConnections() }
      group.addTask { await self.fetchFollowerRequests() }
    }
  }

  public func fetchConnections() async {
    guard let client else { return }
    do {
      let connections: [String] = try await client.get(endpoint: Instances.peers)
      client.addConnections(connections)
    } catch {}
  }

  public func fetchCurrentAccount() async {
    guard let client, client.isAuth else {
      account = nil
      return
    }
    account = Self.accountsCache[client.id]
    if account == nil {
      isLoadingAccount = true
    }
    account = try? await client.get(endpoint: Accounts.verifyCredentials)
    isLoadingAccount = false
    Self.accountsCache[client.id] = account
  }

  public func fetchFollowerRequests() async {
    guard let client else { return }
    do {
      followRequests = try await client.get(endpoint: FollowRequests.list)
    } catch {
      followRequests = []
    }
  }

  public func acceptFollowerRequest(id: String) async {
    guard let client else { return }
    do {
      updatingFollowRequestAccountIds.insert(id)
      defer {
        updatingFollowRequestAccountIds.remove(id)
      }
      _ = try await client.post(endpoint: FollowRequests.accept(id: id))
      await fetchFollowerRequests()
    } catch {}
  }

  public func rejectFollowerRequest(id: String) async {
    guard let client else { return }
    do {
      updatingFollowRequestAccountIds.insert(id)
      defer {
        updatingFollowRequestAccountIds.remove(id)
      }
      _ = try await client.post(endpoint: FollowRequests.reject(id: id))
      await fetchFollowerRequests()
    } catch {}
  }
}
