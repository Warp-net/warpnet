// Modified by Warpnet. See NOTICE.

import Env
import MediaUI
import StatusKit
import SwiftUI

extension WarpnetApp {
  var appScene: some Scene {
    WindowGroup(id: "MainWindow") {
      AppView(selectedTab: $selectedTab, appRouterPath: $appRouterPath)
        .applyTheme(theme)
        .onAppear {
          setNewClientsInEnv(client: appAccountsManager.currentClient)
        }
        .environment(appAccountsManager)
        .environment(appAccountsManager.currentClient)
        .environment(quickLook)
        .environment(currentAccount)
        .environment(currentInstance)
        .environment(userPreferences)
        .environment(theme)
        .environment(watcher)
        .environment(pairing)
        .sheet(item: $quickLook.selectedMediaAttachment) { selectedMediaAttachment in
          MediaUIView(selectedAttachment: selectedMediaAttachment,
                      attachments: quickLook.mediaAttachments)
            .presentationBackground(.ultraThinMaterial)
            .presentationCornerRadius(16)
            .withEnvironments()
        }
    }
    #if targetEnvironment(macCatalyst)
    .defaultSize(width: userPreferences.showiPadSecondaryColumn ? 1100 : 800, height: 1400)
    #endif
    .commands {
      appMenu
    }
    .onChange(of: scenePhase) { _, newValue in
      handleScenePhase(scenePhase: newValue)
    }
    .onChange(of: appAccountsManager.currentClient) { _, newValue in
      setNewClientsInEnv(client: newValue)
      if newValue.isAuth {
        watcher.watch()
      }
    }
  }

  @SceneBuilder
  var otherScenes: some Scene {
    WindowGroup(for: WindowDestinationEditor.self) { destination in
      Group {
        switch destination.wrappedValue {
        case .newStatusEditor:
          StatusEditor.MainView(mode: .new)
        case let .editStatusEditor(status):
          StatusEditor.MainView(mode: .edit(status: status))
        case let .redraftStatusEditor(status):
          StatusEditor.MainView(mode: .redraft(status: status))
        case let .quoteStatusEditor(status):
          StatusEditor.MainView(mode: .quote(status: status))
        case let .replyToStatusEditor(status):
          StatusEditor.MainView(mode: .replyTo(status: status))
        case let .mentionStatusEditor(account):
          StatusEditor.MainView(mode: .mention(account: account))
        case .none:
          EmptyView()
        }
      }
      .withEnvironments()
      .applyTheme(theme)
      .frame(minWidth: 300, minHeight: 400)
    }
    .defaultSize(width: 600, height: 800)
    .windowResizability(.contentMinSize)

    WindowGroup(for: WindowDestinationMedia.self) { destination in
      Group {
        switch destination.wrappedValue {
        case let .mediaViewer(attachments, selectedAttachment):
          MediaUIView(selectedAttachment: selectedAttachment,
                      attachments: attachments)
        case .none:
          EmptyView()
        }
      }
      .withEnvironments()
      .applyTheme(theme)
      .frame(minWidth: 300, minHeight: 400)
    }
    .defaultSize(width: 1200, height: 1000)
    .windowResizability(.contentMinSize)
  }
}
