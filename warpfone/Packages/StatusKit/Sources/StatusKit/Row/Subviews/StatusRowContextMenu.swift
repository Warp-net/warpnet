// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Foundation
import Models
import Network
import SwiftUI

@MainActor
struct StatusRowContextMenu: View {
  @Environment(\.displayScale) var displayScale
  @Environment(\.openWindow) var openWindow
  @Environment(\.isInOwnThread) private var isInOwnThread

  @Environment(Client.self) private var client
  @Environment(SceneDelegate.self) private var sceneDelegate
  @Environment(UserPreferences.self) private var preferences
  @Environment(CurrentAccount.self) private var account
  @Environment(CurrentInstance.self) private var currentInstance
  @Environment(StatusDataController.self) private var statusDataController
  @Environment(QuickLook.self) private var quickLook

  var viewModel: StatusRowViewModel
  @Binding var showTextForSelection: Bool
  @Binding var isBlockConfirmationPresented: Bool

  var boostLabel: some View {
    Label(statusDataController.isReblogged ? "status.action.unboost" : "status.action.boost",
          systemImage: "arrow.2.squarepath")
  }

  var body: some View {
    ControlGroup {
      Button {
        #if targetEnvironment(macCatalyst)
          openWindow(value: WindowDestinationEditor.replyToStatusEditor(status: viewModel.status))
        #else
          viewModel.routerPath.presentedSheet = .replyToStatusEditor(status: viewModel.status)
        #endif
      } label: {
        Label("status.action.reply", systemImage: "arrowshape.turn.up.left")
      }
      Button { Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await statusDataController.toggleFavorite()
      } } label: {
        Label(statusDataController.isFavorited ? "status.action.unfavorite" : "status.action.favorite", systemImage: statusDataController.isFavorited ? "heart.fill" : "heart")
      }
      Button { Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await statusDataController.toggleReblog()
      } } label: {
        boostLabel
      }
      Button { Task {
        HapticManager.shared.fireHaptic(.notification(.success))
        await statusDataController.toggleBookmark()
      } } label: {
        Label(statusDataController.isBookmarked ? "status.action.unbookmark" : "status.action.bookmark",
              systemImage: statusDataController.isBookmarked ? "bookmark.fill" : "bookmark")
      }
    }
    .controlGroupStyle(.compactMenu)
    Button {
      #if targetEnvironment(macCatalyst)
        openWindow(value: WindowDestinationEditor.quoteStatusEditor(status: viewModel.status))
      #else
        viewModel.routerPath.presentedSheet = .quoteStatusEditor(status: viewModel.status)
      #endif
    } label: {
      Label("status.action.quote", systemImage: "quote.bubble")
    }

    Divider()

    Menu("status.action.share-title") {
      if let urlString = viewModel.status.reblog?.url ?? viewModel.status.url,
         let url = URL(string: urlString)
      {
        ShareLink(item: url,
                  subject: Text(viewModel.status.reblog?.account.safeDisplayName ?? viewModel.status.account.safeDisplayName),
                  message: Text(viewModel.status.reblog?.content.asRawText ?? viewModel.status.content.asRawText))
        {
          Label("status.action.share", systemImage: "square.and.arrow.up")
        }

        ShareLink(item: url) {
          Label("status.action.share-link", systemImage: "link")
        }
      } else {
        ShareLink(item: viewModel.status.reblog?.content.asRawText ?? viewModel.status.content.asRawText) {
          Label("status.action.share", systemImage: "square.and.arrow.up")
        }
      }

      Button {
        let view = HStack {
          StatusRowView(viewModel: viewModel)
            .padding(16)
        }
        .environment(\.isInCaptureMode, true)
        .environment(Theme.shared)
        .environment(preferences)
        .environment(account)
        .environment(currentInstance)
        .environment(SceneDelegate())
        .environment(quickLook)
        .environment(viewModel.client)
        .environment(RouterPath())
        .preferredColorScheme(Theme.shared.selectedScheme == .dark ? .dark : .light)
        .foregroundColor(Theme.shared.labelColor)
        .background(Theme.shared.primaryBackgroundColor)
        .frame(width: sceneDelegate.windowWidth - 12)
        .tint(Theme.shared.tintColor)
        let renderer = ImageRenderer(content: view)
        renderer.scale = displayScale
        renderer.isOpaque = false
        if let image = renderer.uiImage {
          viewModel.routerPath.presentedSheet = .shareImage(image: image, status: viewModel.status)
        }
      } label: {
        Label("status.action.share-image", systemImage: "photo")
      }
    }

    Button {
      UIPasteboard.general.string = viewModel.status.reblog?.content.asRawText ?? viewModel.status.content.asRawText
    } label: {
      Label("status.action.copy-text", systemImage: "doc.on.doc")
    }

    Button {
      showTextForSelection = true
    } label: {
      Label("status.action.select-text", systemImage: "selection.pin.in.out")
    }

    if let link = viewModel.status.reblog?.url ?? viewModel.status.url {
      Button {
        UIPasteboard.general.string = link
      } label: {
        Label("status.action.copy-link", systemImage: "link")
      }
    }

    if account.account?.id == viewModel.status.reblog?.account.id ?? viewModel.status.account.id {
      Section("status.action.section.your-post") {
        Button {
          Task {
            if viewModel.isPinned {
              await viewModel.unPin()
            } else {
              await viewModel.pin()
            }
          }
        } label: {
          Label(viewModel.isPinned ? "status.action.unpin" : "status.action.pin", systemImage: viewModel.isPinned ? "pin.fill" : "pin")
        }
        Button {
          #if targetEnvironment(macCatalyst)
            openWindow(value: WindowDestinationEditor.editStatusEditor(status: viewModel.status.reblogAsAsStatus ?? viewModel.status))
          #else
            viewModel.routerPath.presentedSheet = .editStatusEditor(status: viewModel.status.reblogAsAsStatus ?? viewModel.status)
          #endif
        } label: {
          Label("status.action.edit", systemImage: "pencil")
        }
        Button(role: .destructive,
               action: { viewModel.showRedraftAlert = true },
               label: { Label("warpnet.status.action.redraft", systemImage: "pencil.and.scribble") })
        Button(role: .destructive,
               action: { viewModel.showDeleteAlert = true },
               label: { Label("status.action.delete", systemImage: "trash") })
      }
    } else {
      if isInOwnThread,
         viewModel.status.reblog == nil,
         viewModel.status.inReplyToAccountId == account.account?.id
      {
        // The node lets the author of a thread's root remove replies to them.
        Button(role: .destructive,
               action: { viewModel.showDeleteAlert = true },
               label: { Label("status.action.delete", systemImage: "trash") })
      }
      Section(viewModel.status.reblog?.account.acct ?? viewModel.status.account.acct) {
        if viewModel.authorRelationship?.muting == true {
          Button {
            Task {
              do {
                let operationAccount = viewModel.status.reblog?.account ?? viewModel.status.account
                viewModel.authorRelationship = try await client.post(endpoint: Accounts.unmute(id: operationAccount.id))
              } catch { }
            }
          } label: {
            Label("account.action.unmute", systemImage: "speaker")
          }
        } else {
          Button {
            Task {
              do {
                let operationAccount = viewModel.status.reblog?.account ?? viewModel.status.account
                viewModel.authorRelationship = try await client.post(endpoint: Accounts.mute(id: operationAccount.id))
              } catch { }
            }
          } label: {
            Label("account.action.mute", systemImage: "speaker.slash")
          }
        }
        
        #if targetEnvironment(macCatalyst)
          accountContactMenuItems
        #else
        ControlGroup {
          accountContactMenuItems
        }
        #endif
      }
      Section {
        Button(role: .destructive) {
          viewModel.routerPath.presentedSheet = .report(status: viewModel.status.reblogAsAsStatus ?? viewModel.status)
        } label: {
          Label("status.action.report", systemImage: "exclamationmark.bubble")
        }
      }
    }
  }
  
  @ViewBuilder
  private var accountContactMenuItems: some View {
    Button {
      #if targetEnvironment(macCatalyst)
        openWindow(value: WindowDestinationEditor.mentionStatusEditor(account: viewModel.status.reblog?.account ?? viewModel.status.account))
      #else
        viewModel.routerPath.presentedSheet = .mentionStatusEditor(account: viewModel.status.reblog?.account ?? viewModel.status.account)
      #endif
    } label: {
      Label("status.action.mention", systemImage: "at")
    }
    Button {
      Task {
        let account = viewModel.status.reblog?.account ?? viewModel.status.account
        if let conversation: Conversation = try? await client.post(endpoint: Chats.open(accountId: account.id)) {
          viewModel.routerPath.navigate(to: .conversationDetail(conversation: conversation))
        }
      }
    } label: {
      Label("status.action.message", systemImage: "envelope")
    }
    if viewModel.authorRelationship?.blocking == true {
      Button {
        Task {
          do {
            let operationAccount = viewModel.status.reblog?.account ?? viewModel.status.account
            viewModel.authorRelationship = try await client.post(endpoint: Accounts.unblock(id: operationAccount.id))
          } catch { }
        }
      } label: {
        Label("account.action.unblock", systemImage: "person.crop.circle.badge.exclamationmark")
      }
    } else {
      Button {
        isBlockConfirmationPresented = true
      } label: {
        Label("account.action.block", systemImage: "person.crop.circle.badge.xmark")
      }
    }
  }
}

struct SelectTextView: View {
  @Environment(\.dismiss) private var dismiss
  let content: AttributedString

  var body: some View {
    NavigationStack {
      SelectableText(content: content)
        .padding()
        .toolbar {
          ToolbarItem(placement: .navigationBarTrailing) {
            Button {
              dismiss()
            } label: {
              Text("action.done").bold()
            }
          }
        }
        .background(Theme.shared.primaryBackgroundColor)
        .navigationTitle("status.action.select-text")
        .navigationBarTitleDisplayMode(.inline)
    }
  }
}

struct SelectableText: UIViewRepresentable {
  let content: AttributedString

  func makeUIView(context _: Context) -> UITextView {
    let attributedText = NSMutableAttributedString(content)
    attributedText.addAttribute(
      .font,
      value: Font.scaledBodyFont,
      range: NSRange(location: 0, length: content.characters.count)
    )

    let textView = UITextView()
    textView.isEditable = false
    textView.attributedText = attributedText
    textView.textColor = UIColor(Color.label)
    textView.backgroundColor = UIColor(Theme.shared.primaryBackgroundColor)
    return textView
  }

  func updateUIView(_: UITextView, context _: Context) {}
  func makeCoordinator() {}
}
