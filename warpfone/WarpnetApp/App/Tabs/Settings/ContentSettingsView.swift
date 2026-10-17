// Modified by Warpnet. See NOTICE.

import AppAccount
import DesignSystem
import Env
import Models
import Network
import NukeUI
import SwiftUI
import UserNotifications
import Timeline

@MainActor
struct ContentSettingsView: View {
  @Environment(UserPreferences.self) private var userPreferences
  @Environment(Theme.self) private var theme
  
  @State private var contentFilter = TimelineContentFilter.shared

  var body: some View {
    @Bindable var userPreferences = userPreferences
    Form {
      Section("settings.content.media") {
        Toggle(isOn: $userPreferences.autoPlayVideo) {
          Text("settings.other.autoplay-video")
        }
        Toggle(isOn: $userPreferences.muteVideo) {
          Text("settings.other.mute-video")
        }
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif

      Section("settings.content.sharing") {
        Picker("settings.content.sharing.share-button-behavior", selection: $userPreferences.shareButtonBehavior) {
          ForEach(PreferredShareButtonBehavior.allCases, id: \.rawValue) { option in
            Text(option.title)
              .tag(option)
          }
        }
      }
       #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif

      Section {
        Toggle(isOn: $userPreferences.collapseLongPosts) {
          Text("settings.content.collapse-long-posts")
        }
      } header: {
        Text("settings.content.reading")
      } footer: {
        Text("settings.content.collapse-long-posts-hint")
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif

      Section("timeline.content-filter.title") {
        Toggle(isOn: $contentFilter.showBoosts) {
          Label("timeline.filter.show-boosts", image: "Rocket")
        }
        Toggle(isOn: $contentFilter.showReplies) {
          Label("timeline.filter.show-replies", systemImage: "bubble.left.and.bubble.right")
        }
        Toggle(isOn: $contentFilter.showThreads) {
          Label("timeline.filter.show-threads", systemImage: "bubble.left.and.text.bubble.right")
        }
        Toggle(isOn: $contentFilter.showQuotePosts) {
          Label("timeline.filter.show-quote", systemImage: "quote.bubble")
        }
      }
      #if !os(visionOS)
      .listRowBackground(theme.primaryBackgroundColor)
      #endif
    }
    .navigationTitle("settings.content.navigation-title")
    #if !os(visionOS)
    .scrollContentBackground(.hidden)
    .background(theme.secondaryBackgroundColor)
    #endif
  }
}
