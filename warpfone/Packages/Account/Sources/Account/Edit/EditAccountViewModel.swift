// Modified by Warpnet. See NOTICE.

import Models
import Network
import Observation
import SwiftUI
import PhotosUI
import StatusKit

@MainActor
@Observable class EditAccountViewModel {
  public var client: Client?

  var displayName: String = ""
  var note: String = ""
  var website: String = ""
  var avatar: URL?
  var header: URL?
  
  var isPhotoPickerPresented: Bool = false {
    didSet {
      if !isPhotoPickerPresented && mediaPickers.isEmpty {
        isChangingAvatar = false
        isChangingHeader = false
      }
    }
  }
  var isChangingAvatar: Bool = false
  var isChangingHeader: Bool = false

  var isLoading: Bool = true
  var isSaving: Bool = false
  var saveError: Bool = false
  
  var mediaPickers: [PhotosPickerItem] = [] {
    didSet {
      if let item = mediaPickers.first {
        Task {
          if let data = await getItemImageData(item: item) {
            if isChangingAvatar {
              _ = await uploadAvatar(data: data)
            } else if isChangingHeader {
              _ = await uploadHeader(data: data)
            }
            await fetchAccount()
            isChangingAvatar = false
            isChangingHeader = false
            mediaPickers = []
          }
        }
      }
    }
  }

  init() {}

  func fetchAccount() async {
    guard let client else { return }
    do {
      let account: Account = try await client.get(endpoint: Accounts.verifyCredentials)
      displayName = account.displayName ?? ""
      note = account.source?.note ?? ""
      avatar = account.avatar
      header = account.header
      website = account.fields.first?.value.asRawText.escape() ?? ""
      withAnimation {
        isLoading = false
      }
    } catch {}
  }

  func save() async {
    isSaving = true
    do {
      let data = UpdateCredentialsData(displayName: displayName, note: note, website: website)
      let response = try await client?.patch(endpoint: Accounts.updateCredentials(json: data))
      if response?.statusCode != 200 {
        saveError = true
      }
      isSaving = false
    } catch {
      isSaving = false
      saveError = true
    }
  }
  
  private func uploadHeader(data: Data) async -> Bool {
    guard let client else { return false }
    do {
      let response = try await client.mediaUpload(endpoint: Accounts.updateCredentialsMedia,
                                                   mimeType: "image/jpeg",
                                                   filename: "header",
                                                   data: data)
      return response?.statusCode == 200
    } catch {
      return false
    }
  }
  
  private func uploadAvatar(data: Data) async -> Bool {
    guard let client else { return false }
    do {
      let response = try await client.mediaUpload(endpoint: Accounts.updateCredentialsMedia,
                                                   mimeType: "image/jpeg",
                                                   filename: "avatar",
                                                   data: data)
      return response?.statusCode == 200
    } catch {
      return false
    }
  }
  
  private func getItemImageData(item: PhotosPickerItem) async -> Data? {
    guard let imageFile = try? await item.loadTransferable(type: StatusEditor.ImageFileTranseferable.self) else { return nil }

    let compressor = StatusEditor.Compressor()

    guard let compressedData = await compressor.compressImageFrom(url: imageFile.url),
            let image = UIImage(data: compressedData),
            let uploadData = try? await compressor.compressImageForUpload(image)
    else { return nil }
    
    return uploadData
  }
}
