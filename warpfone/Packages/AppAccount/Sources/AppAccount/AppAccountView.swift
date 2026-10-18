// Modified by Warpnet. See NOTICE.

import DesignSystem
import EmojiText
import Env
import SwiftUI

@MainActor
public struct AppAccountView: View {
  @Environment(Theme.self) private var theme
  @Environment(RouterPath.self) private var routerPath
  @Environment(AppAccountsManager.self) private var appAccounts

  @State var viewModel: AppAccountViewModel

  @Binding var isParentPresented: Bool
  
  public init(viewModel: AppAccountViewModel, isParentPresented: Binding<Bool>) {
    self.viewModel = viewModel
    _isParentPresented = isParentPresented
  }

  public var body: some View {
    Group {
      if viewModel.isCompact {
        compactView
      } else {
        fullView
      }
    }
    .onAppear {
      Task {
        await viewModel.fetchAccount()
      }
    }
  }

  @ViewBuilder
  private var compactView: some View {
    HStack {
      if let account = viewModel.account {
        AvatarView(account.avatar)
      } else {
        ProgressView()
      }
    }
  }

  private var fullView: some View {
    Button {
      if let account = viewModel.account {
        if viewModel.isInSettings {
          routerPath.navigate(to: .accountSettingsWithAccount(account: account, appAccount: viewModel.appAccount))
          HapticManager.shared.fireHaptic(.buttonPress)
        } else {
          isParentPresented = false
          routerPath.navigate(to: .accountDetailWithAccount(account: account))
          HapticManager.shared.fireHaptic(.buttonPress)
        }
      }
    } label: {
      HStack {
        if let account = viewModel.account {
          ZStack(alignment: .topTrailing) {
            AvatarView(account.avatar)
            if viewModel.appAccount.id == appAccounts.currentAccount.id {
              Image(systemName: "checkmark.circle.fill")
                .foregroundStyle(.white, .green)
                .offset(x: 5, y: -5)
            }
          }
        } else {
          ProgressView()
          Text(viewModel.appAccount.accountName ?? viewModel.acct)
            .font(.scaledSubheadline)
            .foregroundStyle(Color.secondary)
            .padding(.leading, 6)
        }
        VStack(alignment: .leading) {
          if let account = viewModel.account {
            EmojiTextApp(.init(stringValue: account.safeDisplayName), emojis: account.emojis)
              .foregroundColor(theme.labelColor)
            Text("@\(account.username)")
              .font(.scaledSubheadline)
              .emojiText.size(Font.scaledSubheadlineFont.emojiSize)
              .emojiText.baselineOffset(Font.scaledSubheadlineFont.emojiBaselineOffset)
              .foregroundStyle(Color.secondary)
          }
        }
        if viewModel.isInSettings {
          Spacer()
          Image(systemName: "chevron.right")
            .foregroundStyle(.secondary)
        }
      }
    }
  }
}
