// Modified by Warpnet. See NOTICE.

import AppAccount
import DesignSystem
import Env
import Models
import Network
import SwiftUI

extension StatusEditor {
  @MainActor
  struct EditorView: View {
    @Environment(Theme.self) private var theme
    @Environment(UserPreferences.self) private var preferences
    @Environment(CurrentAccount.self) private var currentAccount
    @Environment(CurrentInstance.self) private var currentInstance
    @Environment(AppAccountsManager.self) private var appAccounts
    @Environment(Client.self) private var client
    #if targetEnvironment(macCatalyst)
      @Environment(\.dismissWindow) private var dismissWindow
    #else
      @Environment(\.dismiss) private var dismiss
    #endif
    
    @Bindable var viewModel: ViewModel
    @Binding var followUpSEVMs: [ViewModel]

    @FocusState<EditorFocusState?>.Binding var editorFocusState: EditorFocusState?
    let assignedFocusState: EditorFocusState
    let isMain: Bool

    var body: some View {
      HStack(spacing: 0) {
        if !isMain {
          Rectangle()
            .fill(theme.tintColor)
            .frame(width: 2)
            .accessibilityHidden(true)
            .padding(.leading, .layoutPadding)
        }

        VStack(spacing: 0) {
          VStack(spacing: 0) {
            accountHeaderView
            textInput
            pollView
            characterCountAndLangView
            if viewModel.mode.isInShareExtension {
              Text("warpnet.share.hint")
                .font(.scaledFootnote)
                .foregroundStyle(.secondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, .layoutPadding)
                .padding(.bottom, 8)
            }
            MediaView(viewModel: viewModel)
            embeddedStatus
          }
          .padding(.vertical)

          Divider()
        }
      }
      #if !os(visionOS)
      .background(theme.primaryBackgroundColor)
      #endif
      .focused($editorFocusState, equals: assignedFocusState)
      .onAppear { setupViewModel() }
    }

    @ViewBuilder
    private var accountHeaderView: some View {
      if let account = currentAccount.account, !viewModel.mode.isEditing {
        HStack {
          if viewModel.mode.isInShareExtension {
            AppAccountsSelectorView(routerPath: RouterPath(),
                                    accountCreationEnabled: false,
                                    avatarConfig: .status)
          } else {
            AvatarView(account.avatar, config: AvatarView.FrameConfig.status)
              .environment(theme)
              .accessibilityHidden(true)
          }

          Text(account.safeDisplayName)
            .font(.scaledSubheadline)
            .fontWeight(.semibold)

          Spacer()

          if case let .followUp(id) = assignedFocusState {
            Button {
              followUpSEVMs.removeAll { $0.id == id }
            } label: {
              HStack {
                Image(systemName: "minus.circle.fill").foregroundStyle(.red)
              }
            }
          }
        }
        .padding(.horizontal, .layoutPadding)
      }
    }

    private var textInput: some View {
      TextView(
        $viewModel.statusText,
        getTextView: { textView in viewModel.textView = textView }
      )
      .placeholder(String(localized: isMain ? "status.editor.text.placeholder" : "status.editor.follow-up.text.placeholder"))
      .setKeyboardType(preferences.isSocialKeyboardEnabled ? .twitter : .default)
      .padding(.horizontal, .layoutPadding)
      .padding(.vertical)
    }

    @ViewBuilder
    private var embeddedStatus: some View {
      if let status = viewModel.replyToStatus {
        Divider().padding(.vertical, .statusComponentSpacing)
        StatusRowView(viewModel: .init(status: status,
                                       client: client,
                                       routerPath: RouterPath(),
                                       showActions: false))
          .accessibilityLabel(status.content.asRawText)
          .environment(RouterPath())
          .allowsHitTesting(false)
          .environment(\.isStatusFocused, false)
          .environment(\.isModal, true)
          .padding(.horizontal, .layoutPadding)
          .padding(.vertical, .statusComponentSpacing)
          #if os(visionOS)
          .background(RoundedRectangle(cornerRadius: 8)
            .foregroundStyle(.background))
          .buttonStyle(.plain)
          .padding(.layoutPadding)
          #endif
        
      } else if let status = viewModel.embeddedStatus {
        StatusEmbeddedView(status: status, client: client, routerPath: RouterPath())
          .padding(.horizontal, .layoutPadding)
          .disabled(true)
      }
    }

    @ViewBuilder
    private var pollView: some View {
      if viewModel.showPoll {
        PollView(viewModel: viewModel, showPoll: $viewModel.showPoll)
          .padding(.horizontal)
      }
    }
    
    
    @ViewBuilder
    private var characterCountAndLangView: some View {
      let value = (currentInstance.instance?.configuration?.statuses.maxCharacters ?? 2048) + viewModel.statusTextCharacterLength
      HStack(alignment: .center) {
        Button {
          withAnimation {
            viewModel.showPoll.toggle()
            viewModel.resetPollDefaults()
          }
        } label: {
          Image(systemName: viewModel.showPoll ? "chart.bar.fill" : "chart.bar")
        }
        .buttonStyle(.bordered)
        .accessibilityLabel("accessibility.editor.button.poll")
        .disabled(viewModel.shouldDisablePollButton)
        .padding(.leading, .layoutPadding)

        Spacer()
        
        Text("\(value)")
          .foregroundColor(value < 0 ? .red : .secondary)
          .font(.callout.monospacedDigit())
          .accessibilityLabel("accessibility.editor.button.characters-remaining")
          .accessibilityValue("\(value)")
          .accessibilityRemoveTraits(.isStaticText)
          .accessibilityAddTraits(.updatesFrequently)
          .accessibilityRespondsToUserInteraction(false)
          .padding(.trailing, .layoutPadding)
      }
      .padding(.vertical, 8)
    }
    
    private func setupViewModel() {
      viewModel.client = client
      viewModel.currentAccount = currentAccount.account
      viewModel.theme = theme
      viewModel.preferences = preferences
      viewModel.prepareStatusText()
      if !client.isAuth, !viewModel.mode.isInShareExtension {
        #if targetEnvironment(macCatalyst)
          dismissWindow()
        #else
          dismiss()
        #endif
        NotificationCenter.default.post(name: .shareSheetClose, object: nil)
      }
    }
  }

}
