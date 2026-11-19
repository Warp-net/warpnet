import SwiftUI
import Env
import AppAccount
import DesignSystem

@MainActor
struct ToolbarTab: ToolbarContent {
  @Environment(\.isSecondaryColumn) private var isSecondaryColumn: Bool
  @Environment(\.horizontalSizeClass) private var horizontalSizeClass
  
  @Environment(UserPreferences.self) private var userPreferences
  @Environment(PairingModel.self) private var pairing
  
  @Binding var routerPath: RouterPath
  
  var body: some ToolbarContent {
    if !isSecondaryColumn {
      StatusEditorToolbarItem()
      ToolbarItem(placement: .navigationBarTrailing) {
        ConnectionIndicator(monitor: pairing.monitor)
      }
      if horizontalSizeClass != .regular {
        ToolbarItem(placement: .navigationBarLeading) {
          AppAccountsSelectorView(routerPath: routerPath)
        }
      }
    }
    if horizontalSizeClass == .regular {
      if (!isSecondaryColumn && !userPreferences.showiPadSecondaryColumn) || isSecondaryColumn {
        SecondaryColumnToolbarItem()
      }
    }
  }
}
