import Foundation

public enum Chats: Endpoint {
  case chats(cursor: String?)
  case messages(chatId: String)
  case open(accountId: String)
  case send(chatId: String, receiverId: String, text: String)
  case delete(chatId: String)
  case deleteMessage(chatId: String, messageId: String)

  public func path() -> String {
    switch self {
    case .chats, .open:
      "warpnet/chats"
    case let .messages(chatId), let .delete(chatId), let .send(chatId, _, _):
      "warpnet/chats/\(chatId)"
    case let .deleteMessage(chatId, messageId):
      "warpnet/chats/\(chatId)/\(messageId)"
    }
  }

  public func queryItems() -> [URLQueryItem]? {
    nil
  }
}
