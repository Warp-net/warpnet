public enum NodeLogLevel: Int, Sendable {
  case debug = 0
  case info = 1
  case warn = 2
  case error = 3
}

public protocol NodeLogSink: Sendable {
  func write(level: NodeLogLevel, component: String, message: String)
}
