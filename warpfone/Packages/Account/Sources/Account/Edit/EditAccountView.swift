// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import Network
import SwiftUI
import NukeUI

@MainActor
public struct EditAccountView: View {
  @Environment(\.dismiss) private var dismiss
  @Environment(Client.self) private var client
  @Environment(Theme.self) private var theme
  @Environment(UserPreferences.self) private var userPrefs

  @State private var viewModel = EditAccountViewModel()
  
  public init() { }

  public var body: some View {
    NavigationStack {
      Form {
        if viewModel.isLoading {
          loadingSection
        } else {
          imagesSection
          aboutSection
          websiteSection
        }
      }
      .environment(\.editMode, .constant(.active))
      #if !os(visionOS)
      .scrollContentBackground(.hidden)
      .background(theme.secondaryBackgroundColor)
      .scrollDismissesKeyboard(.immediately)
      #endif
      .navigationTitle("account.edit.navigation-title")
      .navigationBarTitleDisplayMode(.inline)
      .toolbar {
        toolbarContent
      }
      .alert("account.edit.error.save.title",
             isPresented: $viewModel.saveError,
             actions: {
               Button("alert.button.ok", action: {})
             }, message: { Text("account.edit.error.save.message") })
      .task {
        viewModel.client = client
        await viewModel.fetchAccount()
      }
    }
  }

  private var loadingSection: some View {
    Section {
      HStack {
        Spacer()
        ProgressView()
        Spacer()
      }
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }
  
  private var imagesSection: some View {
    Section {
      ZStack(alignment: .center) {
        if let header = viewModel.header {
          ZStack(alignment: .topLeading) {
            LazyImage(url: header) { state in
              if let image = state.image {
                image
                  .resizable()
                  .aspectRatio(contentMode: .fill)
                  .frame(height: 150)
                  .clipShape(RoundedRectangle(cornerRadius: 8))
                  .clipped()
              } else {
                RoundedRectangle(cornerRadius: 8)
                  .foregroundStyle(theme.primaryBackgroundColor)
                  .frame(height: 150)
              }
            }
            .frame(height: 150)
          }
        }
        if let avatar = viewModel.avatar {
          ZStack(alignment: .bottomLeading) {
            AvatarView(avatar, config: .account)
            Menu {
              Button("account.edit.avatar") {
                viewModel.isChangingAvatar = true
                viewModel.isPhotoPickerPresented = true
              }
              Button("account.edit.header") {
                viewModel.isChangingHeader = true
                viewModel.isPhotoPickerPresented = true
              }
            } label: {
              Image(systemName: "photo.badge.plus")
                .foregroundStyle(.white)
            }
            .buttonStyle(.borderedProminent)
            .clipShape(Circle())
            .offset(x: -8, y: 8)
          }
        }
      }
      .overlay {
        if viewModel.isChangingAvatar || viewModel.isChangingHeader {
          ZStack(alignment: .center) {
            RoundedRectangle(cornerRadius: 8)
              .foregroundStyle(Color.black.opacity(0.40))
            ProgressView()
          }
        }
      }
      .listRowInsets(EdgeInsets())
    }
    .listRowBackground(theme.secondaryBackgroundColor)
    .photosPicker(isPresented: $viewModel.isPhotoPickerPresented,
                  selection: $viewModel.mediaPickers,
                  maxSelectionCount: 1,
                  matching: .any(of: [.images]),
                  photoLibrary: .shared())
  }

  @ViewBuilder
  private var aboutSection: some View {
    Section("account.edit.display-name") {
      TextField("account.edit.display-name", text: $viewModel.displayName)
    }
    .listRowBackground(theme.primaryBackgroundColor)
    Section("account.edit.about") {
      TextField("account.edit.about", text: $viewModel.note, axis: .vertical)
        .frame(maxHeight: 150)
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  private var websiteSection: some View {
    Section("warpnet.account.website") {
      TextField("warpnet.account.website.placeholder", text: $viewModel.website)
        .keyboardType(.URL)
        .textInputAutocapitalization(.never)
        .autocorrectionDisabled()
    }
    #if !os(visionOS)
    .listRowBackground(theme.primaryBackgroundColor)
    #endif
  }

  @ToolbarContentBuilder
  private var toolbarContent: some ToolbarContent {
    CancelToolbarItem()

    ToolbarItem(placement: .navigationBarTrailing) {
      Button {
        Task {
          await viewModel.save()
          dismiss()
        }
      } label: {
        if viewModel.isSaving {
          ProgressView()
        } else {
          Text("action.save").bold()
        }
      }
    }
  }
}
