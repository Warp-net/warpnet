import Foundation

public struct WarpnetEnvelope: Equatable, Sendable {
  public static let version = "0.0.0"

  public let body: String
  public let messageId: String
  public let nodeId: String
  public let path: String
  public let timestampNanos: Int64
  public var signature: String

  public init(body: String,
              nodeId: String,
              path: String,
              timestampNanos: Int64,
              messageId: String = UUID().uuidString.lowercased(),
              signature: String = "")
  {
    self.body = body
    self.nodeId = nodeId
    self.path = path
    self.timestampNanos = timestampNanos
    self.messageId = messageId
    self.signature = signature
  }

  public var timestamp: String {
    WarpnetTimestamp.rfc3339(nanos: timestampNanos)
  }

  public var signingInput: String {
    body + String(timestampNanos) + path
  }

  public func json() -> String {
    "{\"body\":\(body)"
      + ",\"message_id\":\(jsonQuoted(messageId))"
      + ",\"node_id\":\(jsonQuoted(nodeId))"
      + ",\"path\":\(jsonQuoted(path))"
      + ",\"timestamp\":\(jsonQuoted(timestamp))"
      + ",\"version\":\(jsonQuoted(Self.version))"
      + ",\"signature\":\(jsonQuoted(signature))}"
  }
}

public struct WarpnetResponseError: Codable, Equatable, Sendable {
  public let code: Int
  public let message: String
}

public enum WarpnetTimestamp {
  public static func nowNanos() -> Int64 {
    Int64((Date().timeIntervalSince1970 * 1_000_000).rounded(.down)) * 1000
  }

  public static func rfc3339(nanos: Int64) -> String {
    let seconds = floorDivide(nanos, 1_000_000_000)
    let fraction = nanos - seconds * 1_000_000_000
    let days = floorDivide(seconds, 86400)
    let secondOfDay = seconds - days * 86400

    let z = days + 719_468
    let era = floorDivide(z, 146_097)
    let dayOfEra = z - era * 146_097
    let yearOfEra = (dayOfEra - dayOfEra / 1460 + dayOfEra / 36524 - dayOfEra / 146_096) / 365
    let dayOfYear = dayOfEra - (365 * yearOfEra + yearOfEra / 4 - yearOfEra / 100)
    let shiftedMonth = (5 * dayOfYear + 2) / 153
    let day = dayOfYear - (153 * shiftedMonth + 2) / 5 + 1
    let month = shiftedMonth < 10 ? shiftedMonth + 3 : shiftedMonth - 9
    let year = yearOfEra + era * 400 + (month <= 2 ? 1 : 0)

    return "\(pad(year, 4))-\(pad(month, 2))-\(pad(day, 2))"
      + "T\(pad(secondOfDay / 3600, 2)):\(pad(secondOfDay % 3600 / 60, 2)):\(pad(secondOfDay % 60, 2))"
      + ".\(pad(fraction, 9))Z"
  }

  private static func floorDivide(_ value: Int64, _ divisor: Int64) -> Int64 {
    let quotient = value / divisor
    return (value % divisor < 0) ? quotient - 1 : quotient
  }

  private static func pad(_ value: Int64, _ width: Int) -> String {
    let digits = String(value)
    return digits.count >= width ? digits : String(repeating: "0", count: width - digits.count) + digits
  }
}

func jsonQuoted(_ value: String) -> String {
  var out = "\""
  for scalar in value.unicodeScalars {
    switch scalar {
    case "\"":
      out += "\\\""
    case "\\":
      out += "\\\\"
    case "\n":
      out += "\\n"
    case "\r":
      out += "\\r"
    case "\t":
      out += "\\t"
    default:
      if scalar.value < 0x20 {
        let hex = String(scalar.value, radix: 16)
        out += "\\u" + String(repeating: "0", count: 4 - hex.count) + hex
      } else {
        out.unicodeScalars.append(scalar)
      }
    }
  }
  return out + "\""
}
