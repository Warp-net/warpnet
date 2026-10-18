import DesignSystem
import Env
import Models
import Network
import SwiftUI

/// warpdroid's "New message" sheet: search people, pick one, land in the chat
/// the node opens with them. Bridged accounts are left out, as in warpdroid.
@MainActor
struct NewConversationView: View {
  @Environment(\.dismiss) private var dismiss
  @Environment(Client.self) private var client
  @Environment(Theme.self) private var theme

  let onOpen: (Conversation) -> Void

  @State private var query = ""
  @State private var accounts: [Account] = []
  @State private var isSearching = false
  @State private var openingAccountId: String?

  var body: some View {
    NavigationStack {
      List {
        ForEach(accounts) { account in
          Button {
            open(account)
          } label: {
            HStack {
              AvatarView(account.avatar)
              VStack(alignment: .leading) {
                Text(account.safeDisplayName)
                  .font(.scaledSubheadline)
                  .fontWeight(.semibold)
                  .foregroundStyle(theme.labelColor)
                Text("@\(account.acct)")
                  .font(.scaledFootnote)
                  .foregroundStyle(.secondary)
              }
              Spacer()
              if openingAccountId == account.id {
                ProgressView()
              }
            }
          }
          .disabled(openingAccountId != nil)
          .listRowBackground(theme.primaryBackgroundColor)
        }
      }
      .overlay {
        if isSearching, accounts.isEmpty {
          ProgressView()
        }
      }
      .listStyle(.plain)
      .scrollContentBackground(.hidden)
      .background(theme.primaryBackgroundColor)
      .searchable(text: $query,
                  placement: .navigationBarDrawer(displayMode: .always),
                  prompt: Text("warpnet.conversations.new.search"))
      .navigationTitle("warpnet.conversations.new.title")
      .navigationBarTitleDisplayMode(.inline)
      .toolbar {
        CancelToolbarItem()
      }
      .task(id: query) {
        await search()
      }
    }
  }

  private func search() async {
    let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
    guard !trimmed.isEmpty else {
      accounts = []
      return
    }
    try? await Task.sleep(for: .milliseconds(300))
    guard !Task.isCancelled else { return }
    isSearching = true
    defer { isSearching = false }
    let found: [Account] = (try? await client.get(endpoint: Search.accountsSearch(query: trimmed))) ?? []
    guard !Task.isCancelled else { return }
    accounts = found.filter { !$0.acct.contains("@") }
  }

  private func open(_ account: Account) {
    openingAccountId = account.id
    Task {
      defer { openingAccountId = nil }
      if let conversation: Conversation = try? await client.post(endpoint: Chats.open(accountId: account.id)) {
        dismiss()
        onOpen(conversation)
      }
    }
  }
}
