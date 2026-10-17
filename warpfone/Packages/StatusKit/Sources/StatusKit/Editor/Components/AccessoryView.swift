// Modified by Warpnet. See NOTICE.

import DesignSystem
import Env
import Models
import NukeUI
import PhotosUI
import SwiftUI

extension StatusEditor {
  @MainActor
  struct AccessoryView: View {
    @Environment(UserPreferences.self) private var preferences
    @Environment(Theme.self) private var theme
    @Environment(CurrentInstance.self) private var currentInstance

    let focusedSEVM: ViewModel
    @Binding var followUpSEVMs: [ViewModel]

    @State private var isPhotosPickerPresented: Bool = false
    @State private var isFileImporterPresented: Bool = false
    @State private var isCameraPickerPresented: Bool = false

    var body: some View {
      @Bindable var viewModel = focusedSEVM
      #if os(visionOS)
      HStack {
        contentView
          .buttonStyle(.borderless)
      }
      .frame(width: 32)
      .padding(16)
      .glassBackgroundEffect()
      .cornerRadius(8)
      .padding(.trailing, 78)
      #else
      Divider()
      HStack {
        contentView
      }
      .frame(height: 20)
      .padding(.vertical, 12)
      .background(.ultraThickMaterial)
      #endif
    }

    @ViewBuilder
    private var contentView: some View {
      #if os(visionOS)
      VStack(spacing: 8) {
        actionsView
      }
      #else
      ViewThatFits {
        HStack(alignment: .center, spacing: 16) {
          actionsView
        }
        .padding(.horizontal, .layoutPadding)
        
        ScrollView(.horizontal) {
          HStack(alignment: .center, spacing: 16) {
            actionsView
          }
          .padding(.horizontal, .layoutPadding)
        }
        .scrollIndicators(.hidden)
      }
      #endif
    }

    @ViewBuilder
    private var actionsView: some View {
      @Bindable var viewModel = focusedSEVM
      Menu {
        Button {
          isPhotosPickerPresented = true
        } label: {
          Label("status.editor.photo-library", systemImage: "photo")
        }
        #if !targetEnvironment(macCatalyst)
          Button {
            isCameraPickerPresented = true
          } label: {
            Label("status.editor.camera-picker", systemImage: "camera")
          }
        #endif
        Button {
          isFileImporterPresented = true
        } label: {
          Label("status.editor.browse-file", systemImage: "folder")
        }
      } label: {
        if viewModel.isMediasLoading {
          ProgressView()
        } else {
          Image(systemName: "photo.on.rectangle.angled")
        }
      }
      .photosPicker(isPresented: $isPhotosPickerPresented,
                    selection: $viewModel.mediaPickers,
                    maxSelectionCount: currentInstance.instance?.configuration?.statuses.maxMediaAttachments ?? 4,
                    matching: .any(of: [.images, .videos]),
                    photoLibrary: .shared())
      .fileImporter(isPresented: $isFileImporterPresented,
                    allowedContentTypes: [.image, .video, .movie],
                    allowsMultipleSelection: true)
      { result in
        if let urls = try? result.get() {
          viewModel.processURLs(urls: urls)
        }
      }
      .fullScreenCover(isPresented: $isCameraPickerPresented, content: {
        CameraPickerView(selectedImage: .init(get: {
          nil
        }, set: { image in
          if let image {
            viewModel.processCameraPhoto(image: image)
          }
        }))
        .background(.black)
      })
      .accessibilityLabel("accessibility.editor.button.attach-photo")
      .disabled(viewModel.showPoll)

      
      Button {
        // all SEVM have the same visibility value
        followUpSEVMs.append(ViewModel(mode: .new(visibility: focusedSEVM.visibility)))
      } label: {
        Image(systemName: "arrowshape.turn.up.left.circle.fill")
      }
      .disabled(!canAddNewSEVM)

      Spacer()
      
      Button {
        viewModel.insertStatusText(text: "@")
      } label: {
        Image(systemName: "at")
      }
    }

    private var canAddNewSEVM: Bool {
      guard followUpSEVMs.count < 5 else { return false }

      if followUpSEVMs.isEmpty, // there is only mainSEVM on the editor
         !focusedSEVM.statusText.string.isEmpty // focusedSEVM is also mainSEVM
      { return true }

      if let lastSEVMs = followUpSEVMs.last,
         !lastSEVMs.statusText.string.isEmpty
      { return true }

      return false
    }

  }

}
