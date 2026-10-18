import Env
import Models
import SwiftUI

@MainActor
public struct StatusEditorToolbarItem: ToolbarContent {
  @Environment(\.openWindow) private var openWindow
  @Environment(RouterPath.self) private var routerPath

  public init() {}

  public var body: some ToolbarContent {
    ToolbarItem(placement: .navigationBarTrailing) {
      Button {
        Task { @MainActor in
          #if targetEnvironment(macCatalyst)
            openWindow(value: WindowDestinationEditor.newStatusEditor)
          #else
            routerPath.presentedSheet = .newStatusEditor
            HapticManager.shared.fireHaptic(.buttonPress)
          #endif
        }
      } label: {
        Image(systemName: "square.and.pencil")
          .accessibilityLabel("accessibility.tabs.timeline.new-post.label")
          .accessibilityInputLabels([
            LocalizedStringKey("accessibility.tabs.timeline.new-post.label"),
            LocalizedStringKey("accessibility.tabs.timeline.new-post.inputLabel1"),
            LocalizedStringKey("accessibility.tabs.timeline.new-post.inputLabel2"),
          ])
          .offset(y: -2)
      }
    }
  }
}

@MainActor
public struct SecondaryColumnToolbarItem: ToolbarContent {
  @Environment(\.isSecondaryColumn) private var isSecondaryColumn
  @Environment(UserPreferences.self) private var preferences

  public init() {}

  public var body: some ToolbarContent {
    ToolbarItem(placement: isSecondaryColumn ? .navigationBarLeading : .navigationBarTrailing) {
      Button {
        withAnimation {
          preferences.showiPadSecondaryColumn.toggle()
        }
      } label: {
        Image(systemName: "sidebar.right")
      }
    }
  }
}
