// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import Network
import NukeUI
import SwiftUI

@MainActor
struct ConversationMessageView: View {
  @Environment(\.openWindow) private var openWindow
  @Environment(QuickLook.self) private var quickLook
  @Environment(RouterPath.self) private var routerPath
  @Environment(CurrentAccount.self) private var currentAccount
  @Environment(Client.self) private var client
  @Environment(Theme.self) private var theme

  let message: Status
  let conversation: Conversation

  var body: some View {
    let isOwnMessage = message.account.id == currentAccount.account?.id
    VStack {
      HStack(alignment: .bottom) {
        if isOwnMessage {
          Spacer()
        } else {
          AvatarView(message.account.avatar)
            .onTapGesture {
              routerPath.navigate(to: .accountDetailWithAccount(account: message.account))
            }
        }
        VStack(alignment: .leading) {
          EmojiTextApp(message.content, emojis: message.emojis)
            .font(.scaledBody)
            .foregroundColor(theme.labelColor)
            .emojiText.size(Font.scaledBodyFont.emojiSize)
            .emojiText.baselineOffset(Font.scaledBodyFont.emojiBaselineOffset)
            .padding(6)
            .environment(\.openURL, OpenURLAction { url in
              routerPath.handle(url: url)
            })
        }
        .background(isOwnMessage ? theme.tintColor.opacity(0.2) : theme.secondaryBackgroundColor)
        .cornerRadius(8)
        .padding(.leading, isOwnMessage ? 24 : 0)
        .padding(.trailing, isOwnMessage ? 0 : 24)
        .contextMenu {
          contextMenu
        }

        if !isOwnMessage {
          Spacer()
        }
      }

      ForEach(message.mediaAttachments) { media in
        makeMediaView(media)
          .padding(.leading, isOwnMessage ? 24 : 0)
          .padding(.trailing, isOwnMessage ? 0 : 24)
      }

      HStack {
        if isOwnMessage {
          Spacer()
        }
        Group {
          Text(message.createdAt.shortDateFormatted) +
            Text(" ")
          Text(message.createdAt.asDate, style: .time)
        }
        .font(.scaledFootnote)
        .foregroundStyle(.secondary)
        if !isOwnMessage {
          Spacer()
        }
      }
    }
  }

  @ViewBuilder
  private var contextMenu: some View {
    Button {
      UIPasteboard.general.string = message.content.asRawText
    } label: {
      Label("status.action.copy-text", systemImage: "doc.on.doc")
    }
    Divider()
    if message.account.id == currentAccount.account?.id {
      Button("status.action.delete", role: .destructive) {
        Task {
          _ = try? await client.delete(endpoint: Chats.deleteMessage(chatId: conversation.id, messageId: message.id))
          StreamWatcher.shared.emmitDeleteEvent(for: message.id)
        }
      }
    } else {
      Button(role: .destructive) {
        routerPath.presentedSheet = .reportAccount(account: message.account)
      } label: {
        Label("warpnet.report.account-action", systemImage: "exclamationmark.bubble")
      }
    }
  }

  private func makeImageRequest(for url: URL, size: CGSize) -> ImageRequest {
    ImageRequest(url: url, processors: [.resize(size: size)])
  }

  private func mediaWidth(proxy: GeometryProxy) -> CGFloat {
    var width = proxy.frame(in: .local).width
    if UIDevice.current.userInterfaceIdiom == .pad {
      width = width * 0.60
    }
    return width
  }

  private func makeMediaView(_ attachement: MediaAttachment) -> some View {
    GeometryReader { proxy in
      let width = mediaWidth(proxy: proxy)
      if let url = attachement.url {
        LazyImage(request: makeImageRequest(for: url,
                                            size: .init(width: width, height: 200)))
        { state in
          if let image = state.image {
            image
              .resizable()
              .aspectRatio(contentMode: .fill)
              .frame(height: 200)
              .frame(maxWidth: width)
              .clipped()
              .cornerRadius(8)
              .padding(8)
          } else if state.isLoading {
            RoundedRectangle(cornerRadius: 8)
              .fill(Color.gray)
              .frame(height: 200)
          }
        }
      }
    }
    .frame(height: 200)
    .contentShape(Rectangle())
    .onTapGesture {
#if targetEnvironment(macCatalyst)
      openWindow(value: WindowDestinationMedia.mediaViewer(attachments: [attachement],
                                                           selectedAttachment: attachement))
#else
      quickLook.prepareFor(selectedMediaAttachment: attachement, mediaAttachments: [attachement])
#endif
    }
  }
}
