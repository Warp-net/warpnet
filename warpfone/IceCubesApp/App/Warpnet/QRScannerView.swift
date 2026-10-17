import SwiftUI

#if canImport(VisionKit) && !targetEnvironment(macCatalyst) && !os(visionOS)
  import Vision
  import VisionKit

  struct QRScannerView: UIViewControllerRepresentable {
    let onScan: (String) -> Void

    @MainActor static var isAvailable: Bool {
      DataScannerViewController.isSupported && DataScannerViewController.isAvailable
    }

    func makeUIViewController(context: Context) -> DataScannerViewController {
      let controller = DataScannerViewController(recognizedDataTypes: [.barcode(symbologies: [.qr])],
                                                 qualityLevel: .balanced,
                                                 recognizesMultipleItems: false,
                                                 isHighFrameRateTrackingEnabled: false,
                                                 isPinchToZoomEnabled: true,
                                                 isGuidanceEnabled: true,
                                                 isHighlightingEnabled: true)
      controller.delegate = context.coordinator
      try? controller.startScanning()
      return controller
    }

    func updateUIViewController(_: DataScannerViewController, context _: Context) {}

    static func dismantleUIViewController(_ controller: DataScannerViewController, coordinator _: Coordinator) {
      controller.stopScanning()
    }

    func makeCoordinator() -> Coordinator {
      Coordinator(onScan: onScan)
    }

    @MainActor
    final class Coordinator: NSObject, DataScannerViewControllerDelegate {
      private let onScan: (String) -> Void

      init(onScan: @escaping (String) -> Void) {
        self.onScan = onScan
      }

      func dataScanner(_: DataScannerViewController, didAdd addedItems: [RecognizedItem], allItems _: [RecognizedItem]) {
        for item in addedItems {
          if case let .barcode(barcode) = item, let payload = barcode.payloadStringValue {
            onScan(payload)
            return
          }
        }
      }
    }
  }
#else
  struct QRScannerView: View {
    let onScan: (String) -> Void

    static var isAvailable: Bool {
      false
    }

    var body: some View {
      EmptyView()
    }
  }
#endif
