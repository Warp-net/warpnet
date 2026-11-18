// Modified by Warpnet. See NOTICE.

import Env
import Models
import SwiftUI

extension View {
  @MainActor func withSafariRouter() -> some View {
    modifier(SafariRouter())
  }
}

@MainActor
private struct SafariRouter: ViewModifier {
  @Environment(RouterPath.self) private var routerPath

  func body(content: Content) -> some View {
    content
      .environment(\.openURL, OpenURLAction { url in
        // Open internal URL.
        routerPath.handle(url: url)
      })
      .onOpenURL { url in
        // Open external URL (from warpfone://)
        let urlString = url.absoluteString.replacingOccurrences(of: AppInfo.scheme, with: "https://")
        guard let url = URL(string: urlString), url.host != nil else { return }
        _ = routerPath.handle(url: url)
      }
  }
}
