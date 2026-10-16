// Modified by Warpnet. See NOTICE.

import Account
import DesignSystem
import Env
import Models
import Network
import SwiftUI

@MainActor
struct AboutView: View {
  @Environment(RouterPath.self) private var routerPath
  @Environment(Theme.self) private var theme

  let versionNumber: String

  init() {
    if let version = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String {
      versionNumber = version + " "
    } else {
      versionNumber = ""
    }
  }

  var body: some View {
    List {
      Section {
      } footer: {
        Text("\(versionNumber)Based on Ice Cubes ©2023 Thomas Ricouard")
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif

      Section {
        Text("""
        • [EmojiText](https://github.com/divadretlaw/EmojiText)

        • [HTML2Markdown](https://gitlab.com/mflint/HTML2Markdown)

        • [KeychainSwift](https://github.com/evgenyneu/keychain-swift)

        • [LRUCache](https://github.com/nicklockwood/LRUCache)

        • [Bodega](https://github.com/mergesort/Bodega)

        • [Nuke](https://github.com/kean/Nuke)

        • [SwiftSoup](https://github.com/scinfu/SwiftSoup.git)

        • [Atkinson Hyperlegible](https://github.com/googlefonts/atkinson-hyperlegible)

        • [OpenDyslexic](http://opendyslexic.org)

        • [SwiftUI-Introspect](https://github.com/siteline/SwiftUI-Introspect)

        • [SFSafeSymbols](https://github.com/SFSafeSymbols/SFSafeSymbols)
        """)
        .multilineTextAlignment(.leading)
        .font(.scaledSubheadline)
        .foregroundStyle(.secondary)
      } header: {
        Text("settings.about.built-with")
          .textCase(nil)
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
    }
    .listStyle(.insetGrouped)
    #if !os(visionOS)
    .scrollContentBackground(.hidden)
    .background(theme.secondaryBackgroundColor)
    #endif
    .navigationTitle(Text("settings.about.title"))
    .navigationBarTitleDisplayMode(.large)
    .environment(\.openURL, OpenURLAction { url in
      routerPath.handle(url: url)
    })
  }
}

struct AboutView_Previews: PreviewProvider {
  static var previews: some View {
    AboutView()
      .environment(Theme.shared)
  }
}
