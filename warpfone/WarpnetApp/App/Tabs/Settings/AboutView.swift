// Modified by Warpnet. See NOTICE.

import AppAccount
import DesignSystem
import Env
import Models
import Network
import SwiftUI

@MainActor
struct AboutView: View {
  @Environment(RouterPath.self) private var routerPath
  @Environment(Theme.self) private var theme
  @Environment(CurrentAccount.self) private var currentAccount
  @Environment(AppAccountsManager.self) private var appAccounts

  private static let website = URL(string: "https://warpnet.site")!
  private static let issues = URL(string: "https://github.com/Warp-net/warpnet/issues")!

  private let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""

  private var deviceInfo: String {
    "\(UIDevice.current.model)\niOS \(UIDevice.current.systemVersion)"
  }

  private var accountInfo: String? {
    guard let account = currentAccount.account else { return nil }
    let node = appAccounts.currentAccount.server
    return "@\(account.acct)\nNode: \(node)\nVersion: \(version)"
  }

  var body: some View {
    List {
      Section {
        Text("warpnet.about.powered-by")
          .font(.scaledHeadline)
        Link(destination: Self.website) {
          Label("warpnet.about.website", systemImage: "globe")
        }
        Link(destination: Self.issues) {
          Label("warpnet.about.issues", systemImage: "ladybug")
        }
      } header: {
        Text("warpnet.about.version-\(version)")
          .textCase(nil)
      } footer: {
        Text("Based on Ice Cubes ©2023 Thomas Ricouard")
      }
      .listRowBackground(theme.primaryBackgroundColor)

      Section {
        Text(deviceInfo)
          .font(.scaledSubheadline)
          .textSelection(.enabled)
        if let accountInfo {
          Text(accountInfo)
            .font(.scaledSubheadline)
            .textSelection(.enabled)
        }
        Button {
          UIPasteboard.general.string = [deviceInfo, accountInfo].compactMap { $0 }.joined(separator: "\n\n")
        } label: {
          Label("warpnet.about.copy-info", systemImage: "doc.on.doc")
        }
      } header: {
        Text("warpnet.about.device-and-account")
      }
      .listRowBackground(theme.primaryBackgroundColor)

      Section {
        Text("""
        • [ButtonKit](https://github.com/Dean151/ButtonKit)

        • [EmojiText](https://github.com/divadretlaw/EmojiText)

        • [Nuke](https://github.com/kean/Nuke)

        • [SwiftSoup](https://github.com/scinfu/SwiftSoup)

        • [SwiftUI-Introspect](https://github.com/siteline/SwiftUI-Introspect)

        • [go-libp2p](https://github.com/libp2p/go-libp2p)

        • [Atkinson Hyperlegible](https://github.com/googlefonts/atkinson-hyperlegible)

        • [OpenDyslexic](http://opendyslexic.org)
        """)
        .multilineTextAlignment(.leading)
        .font(.scaledSubheadline)
        .foregroundStyle(.secondary)
      } header: {
        Text("settings.about.built-with")
          .textCase(nil)
      }
      .listRowBackground(theme.primaryBackgroundColor)
    }
    .listStyle(.insetGrouped)
    .scrollContentBackground(.hidden)
    .background(theme.secondaryBackgroundColor)
    .navigationTitle(Text("settings.about.title"))
    .navigationBarTitleDisplayMode(.large)
    .environment(\.openURL, OpenURLAction { url in
      routerPath.handle(url: url)
    })
  }
}
