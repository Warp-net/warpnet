// Modified by Warpnet. See NOTICE.

import Models
import Network
import SwiftUI

@MainActor
@Observable class ConversationsListViewModel {
  var client: Client?

  var isLoadingFirstPage: Bool = true
  var isLoadingNextPage: Bool = false
  var conversations: [Conversation] = []
  var isError: Bool = false

  var nextPage: LinkHandler?

  var scrollToTopVisible: Bool = false

  public init() {}

  func fetchConversations() async {
    guard let client else { return }
    if conversations.isEmpty {
      isLoadingFirstPage = true
    }
    do {
      (conversations, nextPage) = try await client.getWithLink(endpoint: Chats.chats(cursor: nil))
      if nextPage?.maxId == nil {
        nextPage = nil
      }
      isError = false
      isLoadingFirstPage = false
    } catch {
      isError = true
      isLoadingFirstPage = false
    }
  }

  func fetchNextPage() async {
    if let maxId = nextPage?.maxId, let client {
      do {
        isLoadingNextPage = true
        var nextMessages: [Conversation] = []
        (nextMessages, nextPage) = try await client.getWithLink(endpoint: Chats.chats(cursor: maxId))
        conversations.append(contentsOf: nextMessages)
        if nextPage?.maxId == nil {
          nextPage = nil
        }
        isLoadingNextPage = false
      } catch {
        isLoadingNextPage = false
      }
    }
  }

  func delete(conversation: Conversation) async {
    guard let client else { return }
    _ = try? await client.delete(endpoint: Chats.delete(chatId: conversation.id))
    await fetchConversations()
  }
}
