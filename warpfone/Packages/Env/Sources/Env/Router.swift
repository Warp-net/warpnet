// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Network
import Observation
import SwiftUI

public enum RouterDestination: Hashable {
  case accountDetail(id: String)
  case accountDetailWithAccount(account: Account)
  case accountSettingsWithAccount(account: Account, appAccount: AppAccount)
  case statusDetail(id: String)
  case statusDetailWithStatus(status: Status)
  case conversationDetail(conversation: Conversation)
  case followers(id: String)
  case following(id: String)
  case favoritedBy(id: String)
  case rebloggedBy(id: String)
  case accountsList(accounts: [Account])
}

public enum WindowDestinationEditor: Hashable, Codable {
  case newStatusEditor
  case editStatusEditor(status: Status)
  case redraftStatusEditor(status: Status)
  case replyToStatusEditor(status: Status)
  case quoteStatusEditor(status: Status)
  case mentionStatusEditor(account: Account)
}

public enum WindowDestinationMedia: Hashable, Codable {
  case mediaViewer(attachments: [MediaAttachment], selectedAttachment: MediaAttachment)
}

public enum SheetDestination: Identifiable {
  case newStatusEditor
  case editStatusEditor(status: Status)
  case redraftStatusEditor(status: Status)
  case replyToStatusEditor(status: Status)
  case quoteStatusEditor(status: Status)
  case mentionStatusEditor(account: Account)
  case settings
  case about
  case report(status: Status)
  case reportAccount(account: Account)
  case shareImage(image: UIImage, status: Status)
  case timelineContentFilter

  public var id: String {
    switch self {
    case .editStatusEditor, .redraftStatusEditor, .newStatusEditor, .replyToStatusEditor, .quoteStatusEditor,
         .mentionStatusEditor:
      "statusEditor"
    case .report, .reportAccount:
      "report"
    case .shareImage:
      "shareImage"
    case .settings, .about:
      "settings"
    case .timelineContentFilter:
      "timelineContentFilter"
    }
  }
}

@MainActor
@Observable public class RouterPath {
  public var client: Client?
  public var urlHandler: ((URL) -> OpenURLAction.Result)?

  public var path: [RouterDestination] = []
  public var presentedSheet: SheetDestination?

  public init() {}

  public func navigate(to: RouterDestination) {
    path.append(to)
  }

  public func handle(url: URL) -> OpenURLAction.Result {
    if url.lastPathComponent.first == "@",
       let host = url.host,
       !host.hasPrefix("www")
    {
      let acct = "\(url.lastPathComponent)@\(host)"
      Task {
        await navigateToAccountFrom(acct: acct, url: url)
      }
      return .handled
    }
    return urlHandler?(url) ?? .systemAction
  }

  public func navigateToAccountFrom(acct: String, url: URL) async {
    guard let client else { return }
    let results: SearchResults? = try? await client.get(endpoint: Search.search(query: acct,
                                                                                type: "accounts"))
    if let account = results?.accounts.first {
      navigate(to: .accountDetailWithAccount(account: account))
    } else {
      _ = await UIApplication.shared.open(url)
    }
  }
}
