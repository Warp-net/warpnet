// Modified by Warpnet. See NOTICE.

import Account
import AppAccount
import DesignSystem
import Env
import Foundation
import Models
import Network
import Nuke
import SwiftUI
import Timeline

@MainActor
struct SettingsTabs: View {
  @Environment(\.dismiss) private var dismiss
  @Environment(\.horizontalSizeClass) private var horizontalSizeClass

  @Environment(PairingModel.self) private var pairing
  @Environment(UserPreferences.self) private var preferences
  @Environment(Client.self) private var client
  @Environment(CurrentInstance.self) private var currentInstance
  @Environment(AppAccountsManager.self) private var appAccountsManager
  @Environment(Theme.self) private var theme

  @State private var routerPath = RouterPath()
  @State private var isUnpairConfirmationPresented = false
  @State private var cachedRemoved = false
  @State private var timelineCache = TimelineCache()

  @Binding var popToRootTab: Tab

  let isModal: Bool

  var body: some View {
    NavigationStack(path: $routerPath.path) {
      Form {
        appSection
        accountsSection
        generalSection
        otherSections
        cacheSection
      }
      .scrollContentBackground(.hidden)
      #if !os(visionOS)
      .background(theme.secondaryBackgroundColor)
      #endif
      .navigationTitle(Text("settings.title"))
      .navigationBarTitleDisplayMode(.inline)
      .toolbarBackground(theme.primaryBackgroundColor.opacity(0.30), for: .navigationBar)
      .toolbar {
        if isModal {
          ToolbarItem {
            Button {
              dismiss()
            } label: {
              Text("action.done").bold()
            }
          }
        }
        if UIDevice.current.userInterfaceIdiom == .pad, !preferences.showiPadSecondaryColumn, !isModal {
          SecondaryColumnToolbarItem()
        }
      }
      .withAppRouter()
      .withSheetDestinations(sheetDestinations: $routerPath.presentedSheet)
    }
    .onAppear {
      routerPath.client = client
    }
    .task {
      if appAccountsManager.currentClient.isAuth {
        await currentInstance.fetchCurrentInstance()
      }
    }
    .withSafariRouter()
    .environment(routerPath)
    .onChange(of: $popToRootTab.wrappedValue) { _, newValue in
      if newValue == .notifications {
        routerPath.path = []
      }
    }
  }

  private var accountsSection: some View {
    Section("settings.section.accounts") {
      if appAccountsManager.currentClient.isAuth {
        AppAccountView(viewModel: .init(appAccount: appAccountsManager.currentAccount), isParentPresented: .constant(false))
      }
      Button(role: .destructive) {
        isUnpairConfirmationPresented = true
      } label: {
        Text("warpnet.pairing.unpair")
      }
      .confirmationDialog("warpnet.pairing.unpair.confirmation",
                          isPresented: $isUnpairConfirmationPresented,
                          titleVisibility: .visible)
      {
        Button("warpnet.pairing.unpair", role: .destructive) {
          Task {
            await timelineCache.clearCache(for: client.id)
            await pairing.unpair()
          }
        }
      }
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  @ViewBuilder
  private var generalSection: some View {
    Section("settings.section.general") {
      NavigationLink(destination: DisplaySettingsView()) {
        Label("settings.general.display", systemImage: "paintpalette")
      }
      if HapticManager.shared.supportsHaptics {
        NavigationLink(destination: HapticSettingsView()) {
          Label("settings.general.haptic", systemImage: "waveform.path")
        }
      }
      NavigationLink(destination: ContentSettingsView()) {
        Label("settings.general.content", systemImage: "rectangle.stack")
      }
      NavigationLink(destination: SwipeActionsSettingsView()) {
        Label("settings.general.swipeactions", systemImage: "hand.draw")
      }
      if UIDevice.current.userInterfaceIdiom == .phone || horizontalSizeClass == .compact {
        NavigationLink(destination: TabbarEntriesSettingsView()) {
          Label("settings.general.tabbarEntries", systemImage: "platter.filled.bottom.iphone")
        }
      } else if UIDevice.current.userInterfaceIdiom == .pad || UIDevice.current.userInterfaceIdiom == .mac {
        NavigationLink(destination: SidebarEntriesSettingsView()) {
          Label("settings.general.sidebarEntries", systemImage: "sidebar.squares.leading")
        }
      }
      #if !targetEnvironment(macCatalyst)
        Link(destination: URL(string: UIApplication.openSettingsURLString)!) {
          Label("settings.system", systemImage: "gear")
        }
        .tint(theme.labelColor)
      #endif
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  @ViewBuilder
  private var otherSections: some View {
    @Bindable var preferences = preferences
    Section {
      #if !targetEnvironment(macCatalyst)
        Picker(selection: $preferences.preferredBrowser) {
          ForEach(PreferredBrowser.allCases, id: \.rawValue) { browser in
            switch browser {
            case .inAppSafari:
              Text("settings.general.browser.in-app").tag(browser)
            case .safari:
              Text("settings.general.browser.system").tag(browser)
            }
          }
        } label: {
          Label("settings.general.browser", systemImage: "network")
        }
        Toggle(isOn: $preferences.inAppBrowserReaderView) {
          Label("settings.general.browser.in-app.readerview", systemImage: "doc.plaintext")
        }
        .disabled(preferences.preferredBrowser != PreferredBrowser.inAppSafari)
      #endif
      Toggle(isOn: $preferences.isSocialKeyboardEnabled) {
        Label("settings.other.social-keyboard", systemImage: "keyboard")
      }
      Toggle(isOn: $preferences.soundEffectEnabled) {
        Label("settings.other.sound-effect", systemImage: "hifispeaker")
      }
      Toggle(isOn: $preferences.fastRefreshEnabled) {
        Label("settings.other.fast-refresh", systemImage: "arrow.clockwise")
      }
    } header: {
      Text("settings.section.other")
    } footer: {
      Text("settings.section.other.footer")
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  private var appSection: some View {
    Section {
      Link(destination: URL(string: AppInfo.weblink)!) {
        Label("settings.app.source", systemImage: "link")
      }
      .accessibilityRemoveTraits(.isButton)
      .tint(theme.labelColor)

      NavigationLink(destination: AboutView()) {
        Label("settings.app.about", systemImage: "info.circle")
      }

    } header: {
      Text("settings.section.app")
    } footer: {
      if let appVersion = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String {
        Text("settings.section.app.footer \(appVersion)").frame(maxWidth: .infinity, alignment: .center)
      }
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  private var cacheSection: some View {
    Section("settings.section.cache") {
      if cachedRemoved {
        Text("action.done")
          .transition(.move(edge: .leading))
      } else {
        Button("settings.cache-media.clear", role: .destructive) {
          ImagePipeline.shared.cache.removeAll()
          withAnimation {
            cachedRemoved = true
          }
        }
      }
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }
}
