// Modified by Warpnet. See NOTICE.

import Env
import Models
import StoreKit
import SwiftUI
import DesignSystem

extension StatusEditor {
  @MainActor
  struct ToolbarItems: ToolbarContent {
    @State private var isDismissAlertPresented: Bool = false
    
    let mainSEVM: ViewModel
    let focusedSEVM: ViewModel
    let followUpSEVMs: [ViewModel]

    @Environment(UserPreferences.self) private var preferences
    @Environment(Theme.self) private var theme
    
    #if targetEnvironment(macCatalyst)
      @Environment(\.dismissWindow) private var dismissWindow
    #else
      @Environment(\.dismiss) private var dismiss
    #endif

    var body: some ToolbarContent {
      ToolbarItem(placement: .navigationBarTrailing) {
        Button {
          Task {
            if mainSEVM.mode.isInShareExtension {
              if await mainSEVM.queueSharedPost() {
                close()
                NotificationCenter.default.post(name: .shareSheetClose, object: nil)
              }
            } else {
              await postAllStatus()
            }
          }
        } label: {
          Image(systemName: "paperplane.fill")
            .bold()
        }
        .buttonStyle(.borderedProminent)
        .disabled(!mainSEVM.canPost || mainSEVM.isPosting)
        .keyboardShortcut(.return, modifiers: .command)
      }

      ToolbarItem(placement: .navigationBarLeading) {
        Button {
          if mainSEVM.shouldDisplayDismissWarning {
            isDismissAlertPresented = true
          } else {
            close()
            NotificationCenter.default.post(name: .shareSheetClose,
                                            object: nil)
          }
        } label: {
          Image(systemName: "xmark")
        }
        .keyboardShortcut(.cancelAction)
        .confirmationDialog(
          "",
          isPresented: $isDismissAlertPresented,
          actions: {
            Button("status.draft.delete", role: .destructive) {
              close()
              NotificationCenter.default.post(name: .shareSheetClose,
                                              object: nil)
            }
            Button("action.cancel", role: .cancel) {}
          }
        )
      }
    }

    @discardableResult
    private func postStatus(with model: ViewModel, isMainPost: Bool) async -> Status? {
      let status = await model.postStatus()
      if status != nil, isMainPost {
        close()
        NotificationCenter.default.post(name: .shareSheetClose, object: nil)
        #if !targetEnvironment(macCatalyst)
          if !mainSEVM.mode.isInShareExtension, !preferences.requestedReview {
            if let scene = UIApplication.shared.connectedScenes.first(where: { $0.activationState == .foregroundActive }) as? UIWindowScene {
              SKStoreReviewController.requestReview(in: scene)
            }
            preferences.requestedReview = true
          }
        #endif
      }

      return status
    }

    private func postAllStatus() async {
      guard var latestPost = await postStatus(with: mainSEVM, isMainPost: true) else { return }
      for p in followUpSEVMs {
        p.mode = .replyTo(status: latestPost)
        guard let post = await postStatus(with: p, isMainPost: false) else {
          break
        }
        latestPost = post
      }
    }

    #if targetEnvironment(macCatalyst)
      private func close() { dismissWindow() }
    #else
      private func close() { dismiss() }
    #endif
  }
}
