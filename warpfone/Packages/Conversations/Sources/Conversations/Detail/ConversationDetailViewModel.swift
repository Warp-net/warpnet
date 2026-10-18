// Modified by Warpnet. See NOTICE.

import Foundation
import Models
import Network
import SwiftUI

@MainActor
@Observable class ConversationDetailViewModel {
  var client: Client?

  var conversation: Conversation

  var isLoadingMessages: Bool = true
  var messages: [Status] = []

  var isSendingMessage: Bool = false
  var newMessageText: String = ""

  init(conversation: Conversation) {
    self.conversation = conversation
  }

  @discardableResult
  func fetchMessages() async -> Bool {
    guard let client else { return false }
    do {
      let fetched: [Status] = try await client.get(endpoint: Chats.messages(chatId: conversation.id))
      isLoadingMessages = false
      let hasNew = fetched.last?.id != messages.last?.id
      messages = fetched
      return hasNew
    } catch {
      isLoadingMessages = false
      return false
    }
  }

  func postMessage() async {
    guard let client, let receiver = conversation.accounts.first else { return }
    isSendingMessage = true
    do {
      let status: Status = try await client.post(endpoint: Chats.send(chatId: conversation.id,
                                                                     receiverId: receiver.id,
                                                                     text: newMessageText))
      appendNewStatus(status: status)
      withAnimation {
        newMessageText = ""
        isSendingMessage = false
      }
    } catch {
      isSendingMessage = false
    }
  }

  func handleEvent(event: any StreamEvent) {
    if let event = event as? StreamEventDelete {
      messages.removeAll { $0.id == event.status }
    }
  }

  private func appendNewStatus(status: Status) {
    if !messages.contains(where: { $0.id == status.id }) {
      messages.append(status)
    }
  }
}
