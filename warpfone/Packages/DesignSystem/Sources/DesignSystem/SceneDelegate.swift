import Combine
import UIKit

@Observable
public class SceneDelegate: NSObject, UIWindowSceneDelegate, Sendable {
  public var window: UIWindow?
  @ObservationIgnored private weak var windowScene: UIWindowScene?
  public private(set) var windowWidth: CGFloat = 0
  public private(set) var windowHeight: CGFloat = 0

  public func scene(_ scene: UIScene,
                    willConnectTo _: UISceneSession,
                    options _: UIScene.ConnectionOptions)
  {
    guard let windowScene = scene as? UIWindowScene else { return }
    self.windowScene = windowScene
    window = windowScene.keyWindow
    windowWidth = windowScene.coordinateSpace.bounds.size.width
    windowHeight = windowScene.coordinateSpace.bounds.size.height

    #if targetEnvironment(macCatalyst)
      if let titlebar = windowScene.titlebar {
        titlebar.titleVisibility = .hidden
        titlebar.toolbar = nil
      }
    #endif
  }

  override public init() {
    super.init()
    Self.observedSceneDelegate.insert(self)
    _ = Self.observer // just for activating the lazy static property
  }

  deinit {
    Task { @MainActor in
      Self.observedSceneDelegate.remove(self)
    }
  }

  private static var observedSceneDelegate: Set<SceneDelegate> = []
  private static let observer = Task { @MainActor in
    while true {
      try? await Task.sleep(for: .seconds(0.1))
      for delegate in observedSceneDelegate {
        let sceneBounds = delegate.windowScene?.coordinateSpace.bounds
        let newWidth = delegate.window?.bounds.size.width ?? sceneBounds?.size.width ?? delegate.windowWidth
        if delegate.windowWidth != newWidth {
          delegate.windowWidth = newWidth
        }
        let newHeight = delegate.window?.bounds.size.height ?? sceneBounds?.size.height ?? delegate.windowHeight
        if delegate.windowHeight != newHeight {
          delegate.windowHeight = newHeight
        }

      }
    }
  }
}
