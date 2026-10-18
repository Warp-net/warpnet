// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import Network
import SwiftUI

@MainActor
struct ConversationsListRow: View {
  @Environment(RouterPath.self) private var routerPath
  @Environment(Theme.self) private var theme

  @Binding var conversation: Conversation
  var viewModel: ConversationsListViewModel

  var body: some View {
    Button {
      routerPath.navigate(to: .conversationDetail(conversation: conversation))
    } label: {
      VStack(alignment: .leading) {
        HStack(alignment: .top, spacing: 8) {
          AvatarView(conversation.accounts.first!.avatar)
            .accessibilityHidden(true)
          VStack(alignment: .leading, spacing: 4) {
            HStack {
              EmojiTextApp(.init(stringValue: conversation.accounts.map(\.safeDisplayName).joined(separator: ", ")),
                           emojis: conversation.accounts.flatMap(\.emojis))
                .font(.scaledSubheadline)
                .foregroundColor(theme.labelColor)
                .emojiText.size(Font.scaledSubheadlineFont.emojiSize)
                .emojiText.baselineOffset(Font.scaledSubheadlineFont.emojiBaselineOffset)
                .fontWeight(.semibold)
                .foregroundColor(theme.labelColor)
                .multilineTextAlignment(.leading)
              Spacer()
              if let message = conversation.lastStatus {
                Text(message.createdAt.relativeFormatted)
                  .font(.scaledFootnote)
              }
            }
            EmojiTextApp(conversation.lastStatus?.content ?? HTMLString(stringValue: ""), emojis: conversation.lastStatus?.emojis ?? [])
              .multilineTextAlignment(.leading)
              .font(.scaledBody)
              .foregroundColor(theme.labelColor)
              .emojiText.size(Font.scaledBodyFont.emojiSize)
              .emojiText.baselineOffset(Font.scaledBodyFont.emojiBaselineOffset)
              .accessibilityLabel(conversation.lastStatus?.content.asRawText ?? "")
          }
          Spacer()
        }
        .padding(.vertical, 4)
      }
      .contextMenu {
        contextMenu
      }
      .accessibilityElement(children: .combine)
      .accessibilityActions {
        contextMenu
      }
    }
    .buttonStyle(.plain)
    .hoverEffectDisabled()
  }

  @ViewBuilder
  private var contextMenu: some View {
    if let message = conversation.lastStatus {
      Button {
        UIPasteboard.general.string = message.content.asRawText
      } label: {
        Label("status.action.copy-text", systemImage: "doc.on.doc")
      }
    }
    if let account = conversation.accounts.first {
      Button(role: .destructive) {
        routerPath.presentedSheet = .reportAccount(account: account)
      } label: {
        Label("warpnet.report.account-action", systemImage: "exclamationmark.bubble")
      }
    }
    Button(role: .destructive) {
      Task {
        await viewModel.delete(conversation: conversation)
      }
    } label: {
      Label("conversations.action.delete", systemImage: "trash")
    }
  }
}
