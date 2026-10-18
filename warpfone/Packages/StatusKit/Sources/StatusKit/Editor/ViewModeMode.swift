import Models
import SwiftUI
import UIKit

public extension StatusEditor.ViewModel {
  enum Mode {
    case replyTo(status: Status)
    case new
    case edit(status: Status)
    case redraft(status: Status)
    case quote(status: Status)
    case mention(account: Account)
    case shareExtension(items: [NSItemProvider])

    var isInShareExtension: Bool {
      switch self {
      case .shareExtension:
        true
      default:
        false
      }
    }

    var isEditing: Bool {
      switch self {
      case .edit:
        true
      default:
        false
      }
    }

    var replyToStatus: Status? {
      switch self {
      case let .replyTo(status):
        status
      default:
        nil
      }
    }

    var inReplyToId: String? {
      switch self {
      case let .replyTo(status):
        status.id
      case let .redraft(status):
        status.inReplyToId
      default:
        nil
      }
    }

    var title: LocalizedStringKey {
      switch self {
      case .new, .mention, .shareExtension, .redraft:
        "status.editor.mode.new"
      case .edit:
        "status.editor.mode.edit"
      case let .replyTo(status):
        "status.editor.mode.reply-\(status.reblog?.account.displayNameWithoutEmojis ?? status.account.displayNameWithoutEmojis)"
      case let .quote(status):
        "status.editor.mode.quote-\(status.reblog?.account.displayNameWithoutEmojis ?? status.account.displayNameWithoutEmojis)"
      }
    }
  }
}
