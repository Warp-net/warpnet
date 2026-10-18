// Modified by Warpnet. See NOTICE.

import Combine
import DesignSystem
import Env
import Models
import Network
import PhotosUI
import SwiftUI

extension StatusEditor {
  
  @MainActor
  @Observable public class ViewModel: NSObject, Identifiable {
    public let id = UUID()

    var mode: Mode

    var client: Client?
    var currentAccount: Account? {
      didSet {
        if let itemsProvider {
          mediaContainers = []
          processItemsProvider(items: itemsProvider)
        }
      }
    }

    var theme: Theme?
    var preferences: UserPreferences?

    var textView: UITextView? {
      didSet {
        textView?.pasteDelegate = self
      }
    }

    var selectedRange: NSRange {
      get {
        guard let textView else {
          return .init(location: 0, length: 0)
        }
        return textView.selectedRange
      }
      set {
        textView?.selectedRange = newValue
      }
    }

    var markedTextRange: UITextRange? {
      guard let textView else {
        return nil
      }
      return textView.markedTextRange
    }

    var statusText = NSMutableAttributedString(string: "") {
      didSet {
        let range = selectedRange
        processText()
        textView?.attributedText = statusText
        selectedRange = range
      }
    }

    var statusTextCharacterLength: Int {
      -statusText.string.utf8.count
    }

    private var itemsProvider: [NSItemProvider]?

    var showPoll: Bool = false
    var pollDuration = Duration.oneDay
    var pollOptions: [String] = ["", ""]

    var postingProgress: Double = 0.0
    var postingTimer: Timer?
    var isPosting: Bool = false
    
    var mediaPickers: [PhotosPickerItem] = [] {
      didSet {
        if mediaPickers.count > 4 {
          mediaPickers = mediaPickers.prefix(4).map { $0 }
        }

        let removedIDs = oldValue
          .filter { !mediaPickers.contains($0) }
          .compactMap(\.itemIdentifier)
        mediaContainers.removeAll { removedIDs.contains($0.id) }

        let newPickerItems = mediaPickers.filter { !oldValue.contains($0) }
        if !newPickerItems.isEmpty {
          isMediasLoading = true
          for item in newPickerItems {
            prepareToPost(for: item)
          }
        }
      }
    }

    var isMediasLoading: Bool = false

    var mediaContainers: [MediaContainer] = []
    var replyToStatus: Status?
    var embeddedStatus: Status?

    var postingError: String?
    var showPostingErrorAlert: Bool = false

    var canPost: Bool {
      let length = statusText.string.utf8.count
      return length > 0 && length <= (CurrentInstance.shared.instance?.configuration?.statuses.maxCharacters ?? 2048)
    }

    var shouldDisablePollButton: Bool {
      !mediaContainers.isEmpty || mode.isInShareExtension
    }

    var shouldDisplayDismissWarning: Bool {
      var modifiedStatusText = statusText.string.trimmingCharacters(in: .whitespaces)

      if let mentionString, modifiedStatusText.hasPrefix(mentionString) {
        modifiedStatusText = String(modifiedStatusText.dropFirst(mentionString.count))
      }

      return !modifiedStatusText.isEmpty && !mode.isInShareExtension
    }

    var mentionsSuggestions: [Account] = []
    private var currentSuggestionRange: NSRange?

    private var quotedStatusId: String? {
      if case let .quote(status) = mode {
        return status.id
      }
      return nil
    }

    private var mentionString: String?

    private var suggestedTask: Task<Void, Never>?

    init(mode: Mode) {
      self.mode = mode
    }

    func postStatus() async -> Status? {
      guard let client else { return nil }
      do {
        if postingTimer == nil {
          Timer.scheduledTimer(withTimeInterval: 0.05, repeats: true) { timer in
            Task { @MainActor in
              if self.postingProgress < 100 {
                self.postingProgress += 0.5
              }
              if self.postingProgress >= 100 {
                self.postingTimer?.invalidate()
                self.postingTimer = nil
              }
            }
          }
        }
        isPosting = true
        let postStatus: Status?
        var pollData: StatusData.PollData?
        if let pollOptions = getPollOptionsForAPI() {
          pollData = .init(options: pollOptions, expires_in: pollDuration.rawValue)
        }
        let data = StatusData(status: statusText.string,
                              inReplyToId: mode.inReplyToId,
                              mediaIds: mediaContainers.compactMap { $0.mediaAttachment?.id },
                              poll: pollData,
                              quotedStatusId: quotedStatusId)
        switch mode {
        case .new, .replyTo, .quote, .mention, .shareExtension, .redraft:
          postStatus = try await client.post(endpoint: Statuses.postStatus(json: data))
          if let postStatus {
            StreamWatcher.shared.emmitPostEvent(for: postStatus)
          }
        case let .edit(status):
          postStatus = try await client.put(endpoint: Statuses.editStatus(id: status.id, json: data))
          if let postStatus {
            StreamWatcher.shared.emmitEditEvent(for: postStatus)
          }
        }
        
        postingTimer?.invalidate()
        postingTimer = nil
        
        withAnimation {
          postingProgress = 99.0
        }
        try await Task.sleep(for: .seconds(0.5))
        HapticManager.shared.fireHaptic(.notification(.success))
        isPosting = false
        
        return postStatus
      } catch {
        if let error = error as? Models.ServerError {
          postingError = error.error
          showPostingErrorAlert = true
        } else if let postError = error as? PostError {
          postingError = postError.description
          showPostingErrorAlert = true
        } else if !(error is CancellationError) {
          postingError = error.localizedDescription
          showPostingErrorAlert = true
        }
        isPosting = false
        HapticManager.shared.fireHaptic(.notification(.error))
        return nil
      }
    }

    // MARK: - Status Text manipulations

    func insertStatusText(text: String) {
      let string = statusText
      string.mutableString.insert(text, at: selectedRange.location)
      statusText = string
      selectedRange = NSRange(location: selectedRange.location + text.utf16.count, length: 0)
      processText()
    }

    func replaceTextWith(text: String, inRange: NSRange) {
      let string = statusText
      string.mutableString.deleteCharacters(in: inRange)
      string.mutableString.insert(text, at: inRange.location)
      statusText = string
      selectedRange = NSRange(location: inRange.location + text.utf16.count, length: 0)
    }

    func replaceTextWith(text: String) {
      statusText = .init(string: text)
      selectedRange = .init(location: text.utf16.count, length: 0)
    }

    func prepareStatusText() {
      switch mode {
      case .new:
        break
      case let .shareExtension(items):
        itemsProvider = items
        processItemsProvider(items: items)
      case let .replyTo(status):
        var mentionString = ""
        if (status.reblog?.account.acct ?? status.account.acct) != currentAccount?.acct {
          mentionString = "@\(status.reblog?.account.acct ?? status.account.acct)"
        }
        for mention in status.mentions where mention.acct != currentAccount?.acct {
          if !mentionString.isEmpty {
            mentionString += " "
          }
          mentionString += "@\(mention.acct)"
        }
        if !mentionString.isEmpty {
          mentionString += " "
        }
        replyToStatus = status
        statusText = .init(string: mentionString)
        selectedRange = .init(location: mentionString.utf16.count, length: 0)
        if !mentionString.isEmpty {
          self.mentionString = mentionString.trimmingCharacters(in: .whitespaces)
        }
      case let .mention(account):
        statusText = .init(string: "@\(account.acct) ")
        selectedRange = .init(location: statusText.string.utf16.count, length: 0)
      case let .edit(status), let .redraft(status):
        var rawText = status.content.asRawText.escape()
        for mention in status.mentions {
          rawText = rawText.replacingOccurrences(of: "@\(mention.username)", with: "@\(mention.acct)")
        }
        statusText = .init(string: rawText)
        selectedRange = .init(location: statusText.string.utf16.count, length: 0)
        mediaContainers = status.mediaAttachments.map {
          MediaContainer(
            id: UUID().uuidString,
            image: nil,
            movieTransferable: nil,
            gifTransferable: nil,
            mediaAttachment: $0,
            error: nil
          )
        }
      case let .quote(status):
        embeddedStatus = status
      }
    }

    private func processText() {
      guard markedTextRange == nil else { return }
      statusText.addAttributes([.foregroundColor: UIColor(Theme.shared.labelColor),
                                .font: Font.scaledBodyUIFont,
                                .backgroundColor: UIColor.clear,
                                .underlineColor: UIColor.clear],
                               range: NSMakeRange(0, statusText.string.utf16.count))
      let mentionPattern = "(@+[a-zA-Z0-9(_).-]{1,})"
      let urlPattern = "(?i)https?://(?:www\\.)?\\S+(?:/|\\b)"

      do {
        let mentionRegex = try NSRegularExpression(pattern: mentionPattern, options: [])
        let urlRegex = try NSRegularExpression(pattern: urlPattern, options: [])

        let range = NSMakeRange(0, statusText.string.utf16.count)
        let ranges = mentionRegex.matches(in: statusText.string,
                                          options: [],
                                          range: range).map(\.range)

        let urlRanges = urlRegex.matches(in: statusText.string,
                                         options: [],
                                         range: range).map(\.range)

        var foundSuggestionRange = false
        for nsRange in ranges {
          statusText.addAttributes([.foregroundColor: UIColor(theme?.tintColor ?? .brand)],
                                   range: nsRange)
          if selectedRange.location == (nsRange.location + nsRange.length),
             let range = Range(nsRange, in: statusText.string)
          {
            foundSuggestionRange = true
            currentSuggestionRange = nsRange
            loadAutoCompleteResults(query: String(statusText.string[range]))
          }
        }

        if !foundSuggestionRange || ranges.isEmpty {
          resetAutoCompletion()
        }

        for range in urlRanges {
          statusText.addAttributes([.foregroundColor: UIColor(theme?.tintColor ?? .brand),
                                    .underlineStyle: NSUnderlineStyle.single.rawValue,
                                    .underlineColor: UIColor(theme?.tintColor ?? .brand)],
                                   range: NSRange(location: range.location, length: range.length))
        }

        statusText.enumerateAttributes(in: range) { attributes, range, _ in
          if attributes[.link] != nil {
            statusText.removeAttribute(.link, range: range)
          }
        }
      } catch {}
    }

    // MARK: - Shar sheet / Item provider

    func processURLs(urls: [URL]) {
      isMediasLoading = true
      let items = urls.filter { $0.startAccessingSecurityScopedResource() }
        .compactMap { NSItemProvider(contentsOf: $0) }
      processItemsProvider(items: items)
    }

    func processGIFData(data: Data) {
      isMediasLoading = true
      let url = URL.temporaryDirectory.appending(path: "\(UUID().uuidString).gif")
      try? data.write(to: url)
      let container = MediaContainer(id: UUID().uuidString,
                                     image: nil,
                                     movieTransferable: nil,
                                     gifTransferable: .init(url: url),
                                     mediaAttachment: nil,
                                     error: nil)
      prepareToPost(for: container)
    }

    func processCameraPhoto(image: UIImage) {
      let container = MediaContainer(
        id: UUID().uuidString,
        image: image,
        movieTransferable: nil,
        gifTransferable: nil,
        mediaAttachment: nil,
        error: nil
      )
      prepareToPost(for: container)
    }

    private func processItemsProvider(items: [NSItemProvider]) {
      Task {
        var initialText: String = ""
        for item in items {
          if let identifier = item.registeredTypeIdentifiers.first {
            let handledItemType = UTTypeSupported(value: identifier)
            do {
              let compressor = Compressor()
              let content = try await handledItemType.loadItemContent(item: item)
              if let text = content as? String {
                initialText += "\(text) "
              } else if let image = content as? UIImage {
                let container = MediaContainer(
                  id: UUID().uuidString,
                  image: image,
                  movieTransferable: nil,
                  gifTransferable: nil,
                  mediaAttachment: nil,
                  error: nil
                )
                prepareToPost(for: container)
              } else if let content = content as? ImageFileTranseferable,
                        let compressedData = await compressor.compressImageFrom(url: content.url),
                        let image = UIImage(data: compressedData)
              {
                let container = MediaContainer(
                  id: UUID().uuidString,
                  image: image,
                  movieTransferable: nil,
                  gifTransferable: nil,
                  mediaAttachment: nil,
                  error: nil
                )
                prepareToPost(for: container)
              } else if let video = content as? MovieFileTranseferable {
                let container = MediaContainer(
                  id: UUID().uuidString,
                  image: nil,
                  movieTransferable: video,
                  gifTransferable: nil,
                  mediaAttachment: nil,
                  error: nil
                )
                prepareToPost(for: container)
              } else if let gif = content as? GifFileTranseferable {
                let container = MediaContainer(
                  id: UUID().uuidString,
                  image: nil,
                  movieTransferable: nil,
                  gifTransferable: gif,
                  mediaAttachment: nil,
                  error: nil
                )
                prepareToPost(for: container)
              }
            } catch {
              isMediasLoading = false
            }
          }
        }
        if !initialText.isEmpty {
          statusText = .init(string: initialText)
          selectedRange = .init(location: statusText.string.utf16.count, length: 0)
        }
      }
    }

    // MARK: - Polls

    func resetPollDefaults() {
      pollOptions = ["", ""]
      pollDuration = .oneDay
    }

    private func getPollOptionsForAPI() -> [String]? {
      let options = pollOptions.filter { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
      return options.isEmpty ? nil : options
    }

    // MARK: - Autocomplete

    private func loadAutoCompleteResults(query: String) {
      guard let client else { return }
      var query = query
      suggestedTask?.cancel()
      suggestedTask = Task {
        do {
          switch query.first {
          case "@":
            guard query.utf8.count > 1 else { return }
            query.removeFirst()
            let accounts: [Account] = try await client.get(endpoint: Search.accountsSearch(query: query))
            guard !Task.isCancelled else {
              return
            }
            withAnimation {
              mentionsSuggestions = accounts
            }
          default:
            break
          }
        } catch {}
      }
    }

    private func resetAutoCompletion() {
      if !mentionsSuggestions.isEmpty ||
          currentSuggestionRange != nil {
        withAnimation {
          mentionsSuggestions = []
          currentSuggestionRange = nil
        }
      }
    }

    func selectMentionSuggestion(account: Account) {
      if let range = currentSuggestionRange {
        replaceTextWith(text: "@\(account.acct) ", inRange: range)
      }
    }

    // MARK: - Media related function

    private func indexOf(container: MediaContainer) -> Int? {
      mediaContainers.firstIndex(where: { $0.id == container.id })
    }

    func prepareToPost(for pickerItem: PhotosPickerItem) {
      Task(priority: .high) {
        if let container = await makeMediaContainer(from: pickerItem) {
          self.mediaContainers.append(container)
          await upload(container: container)
          self.isMediasLoading = false
        }
      }
    }

    func prepareToPost(for container: MediaContainer) {
      Task(priority: .high) {
        self.mediaContainers.append(container)
        await upload(container: container)
        self.isMediasLoading = false
      }
    }

    nonisolated func makeMediaContainer(from pickerItem: PhotosPickerItem) async -> MediaContainer? {
      await withTaskGroup(of: MediaContainer?.self, returning: MediaContainer?.self) { taskGroup in
        taskGroup.addTask(priority: .high) { await Self.makeImageContainer(from: pickerItem) }
        taskGroup.addTask(priority: .high) { await Self.makeGifContainer(from: pickerItem) }
        taskGroup.addTask(priority: .high) { await Self.makeMovieContainer(from: pickerItem) }

        for await container in taskGroup {
          if let container {
            taskGroup.cancelAll()
            return container
          }
        }

        return nil
      }
    }

    private static func makeGifContainer(from pickerItem: PhotosPickerItem) async -> MediaContainer? {
      guard let gifFile = try? await pickerItem.loadTransferable(type: GifFileTranseferable.self) else { return nil }

      return MediaContainer(
        id: pickerItem.itemIdentifier ?? UUID().uuidString,
        image: nil,
        movieTransferable: nil,
        gifTransferable: gifFile,
        mediaAttachment: nil,
        error: nil
      )
    }

    private static func makeMovieContainer(from pickerItem: PhotosPickerItem) async -> MediaContainer? {
      guard let movieFile = try? await pickerItem.loadTransferable(type: MovieFileTranseferable.self) else { return nil }

      return MediaContainer(
        id: pickerItem.itemIdentifier ?? UUID().uuidString,
        image: nil,
        movieTransferable: movieFile,
        gifTransferable: nil,
        mediaAttachment: nil,
        error: nil
      )
    }

    private static func makeImageContainer(from pickerItem: PhotosPickerItem) async -> MediaContainer? {
      guard let imageFile = try? await pickerItem.loadTransferable(type: ImageFileTranseferable.self) else { return nil }

      let compressor = Compressor()

      guard let compressedData = await compressor.compressImageFrom(url: imageFile.url),
            let image = UIImage(data: compressedData)
      else { return nil }

      return MediaContainer(
        id: pickerItem.itemIdentifier ?? UUID().uuidString,
        image: image,
        movieTransferable: nil,
        gifTransferable: nil,
        mediaAttachment: nil,
        error: nil
      )
    }

    func upload(container: MediaContainer) async {
      guard !mode.isInShareExtension else { return }
      if let index = indexOf(container: container) {
        let originalContainer = mediaContainers[index]
        guard originalContainer.mediaAttachment == nil else { return }
        let newContainer = MediaContainer(
          id: originalContainer.id,
          image: originalContainer.image,
          movieTransferable: originalContainer.movieTransferable,
          gifTransferable: nil,
          mediaAttachment: nil,
          error: nil
        )
        mediaContainers[index] = newContainer
        do {
          let compressor = Compressor()
          if let image = originalContainer.image {
            let imageData = try await compressor.compressImageForUpload(image)
            let uploadedMedia = try await uploadMedia(data: imageData, mimeType: "image/jpeg")
            if let index = indexOf(container: newContainer) {
              mediaContainers[index] = MediaContainer(
                id: originalContainer.id,
                image: mode.isInShareExtension ? originalContainer.image : nil,
                movieTransferable: nil,
                gifTransferable: nil,
                mediaAttachment: uploadedMedia,
                error: nil
              )
            }
          } else if let videoURL = originalContainer.movieTransferable?.url,
                    let compressedVideoURL = await compressor.compressVideo(videoURL),
                    let data = try? Data(contentsOf: compressedVideoURL)
          {
            let uploadedMedia = try await uploadMedia(data: data, mimeType: compressedVideoURL.mimeType())
            if let index = indexOf(container: newContainer) {
              mediaContainers[index] = MediaContainer(
                id: originalContainer.id,
                image: mode.isInShareExtension ? originalContainer.image : nil,
                movieTransferable: originalContainer.movieTransferable,
                gifTransferable: nil,
                mediaAttachment: uploadedMedia,
                error: nil
              )
            }
          } else if let gifData = originalContainer.gifTransferable?.data {
            let uploadedMedia = try await uploadMedia(data: gifData, mimeType: "image/gif")
            if let index = indexOf(container: newContainer) {
              mediaContainers[index] = MediaContainer(
                id: originalContainer.id,
                image: mode.isInShareExtension ? originalContainer.image : nil,
                movieTransferable: nil,
                gifTransferable: originalContainer.gifTransferable,
                mediaAttachment: uploadedMedia,
                error: nil
              )
            }
          }
        } catch {
          if let index = indexOf(container: newContainer) {
            mediaContainers[index] = MediaContainer(
              id: originalContainer.id,
              image: originalContainer.image,
              movieTransferable: nil,
              gifTransferable: nil,
              mediaAttachment: nil,
              error: error
            )
          }
        }
      }
    }

    func queueSharedPost() async -> Bool {
      do {
        let directory = SharedStore.directory
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let compressor = Compressor()
        var files: [String] = []
        for container in mediaContainers {
          if let image = container.image {
            let name = "\(UUID().uuidString).jpg"
            try await compressor.compressImageForUpload(image).write(to: directory.appending(path: name))
            files.append(name)
          } else if let url = container.movieTransferable?.url,
                    let compressed = await compressor.compressVideo(url)
          {
            let name = "\(UUID().uuidString).\(compressed.pathExtension.isEmpty ? "mp4" : compressed.pathExtension)"
            try FileManager.default.copyItem(at: compressed, to: directory.appending(path: name))
            files.append(name)
          } else if let gif = container.gifTransferable?.data {
            let name = "\(UUID().uuidString).gif"
            try gif.write(to: directory.appending(path: name))
            files.append(name)
          }
        }
        try SharedStore.enqueue(SharedPost(content: statusText.string, mediaFiles: files))
        return true
      } catch {
        postingError = error.localizedDescription
        showPostingErrorAlert = true
        return false
      }
    }

    private func uploadMedia(data: Data, mimeType: String) async throws -> MediaAttachment? {
      guard let client else { return nil }
      return try await client.mediaUpload(endpoint: Media.medias,
                                          mimeType: mimeType,
                                          filename: "file",
                                          data: data)
    }
  }
}

// MARK: - DropDelegate

extension StatusEditor.ViewModel: DropDelegate {
  public func performDrop(info: DropInfo) -> Bool {
    let item = info.itemProviders(for: [.image, .video, .gif, .mpeg4Movie, .quickTimeMovie, .movie])
    processItemsProvider(items: item)
    return true
  }
}

// MARK: - UITextPasteDelegate

extension StatusEditor.ViewModel: UITextPasteDelegate {
  public func textPasteConfigurationSupporting(
    _: UITextPasteConfigurationSupporting,
    transform item: UITextPasteItem
  ) {
    if !item.itemProvider.registeredContentTypes(conformingTo: .image).isEmpty ||
      !item.itemProvider.registeredContentTypes(conformingTo: .video).isEmpty ||
      !item.itemProvider.registeredContentTypes(conformingTo: .gif).isEmpty
    {
      processItemsProvider(items: [item.itemProvider])
      item.setNoResult()
    } else {
      item.setDefaultResult()
    }
  }
}

extension PhotosPickerItem: @unchecked Sendable {}
