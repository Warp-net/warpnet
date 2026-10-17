import Foundation

public enum Base45 {
  public enum DecodingError: Error, Equatable {
    case invalidLength
    case invalidCharacter
    case valueOutOfRange
  }

  private static let alphabet = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:".utf8)

  private static let decodeTable: [Int] = {
    var table = [Int](repeating: -1, count: 256)
    for (index, char) in alphabet.enumerated() {
      table[Int(char)] = index
    }
    return table
  }()

  public static func encode(_ data: Data) -> String {
    let bytes = [UInt8](data)
    var out = [UInt8]()
    out.reserveCapacity(bytes.count / 2 * 3 + 2)
    var index = 0
    while index + 1 < bytes.count {
      let value = Int(bytes[index]) << 8 | Int(bytes[index + 1])
      out.append(alphabet[value % 45])
      out.append(alphabet[value / 45 % 45])
      out.append(alphabet[value / 2025])
      index += 2
    }
    if index < bytes.count {
      let value = Int(bytes[index])
      out.append(alphabet[value % 45])
      out.append(alphabet[value / 45])
    }
    return String(decoding: out, as: UTF8.self)
  }

  public static func decode(_ string: String) throws -> Data {
    let chars = Array(string.utf8)
    if chars.isEmpty { return Data() }
    let remainder = chars.count % 3
    guard remainder != 1 else { throw DecodingError.invalidLength }
    var out = Data(capacity: chars.count / 3 * 2 + 1)
    var index = 0
    while index + 2 < chars.count {
      let value = try lookup(chars[index]) + lookup(chars[index + 1]) * 45 + lookup(chars[index + 2]) * 2025
      guard value <= 0xFFFF else { throw DecodingError.valueOutOfRange }
      out.append(UInt8(value >> 8))
      out.append(UInt8(value & 0xFF))
      index += 3
    }
    if remainder == 2 {
      let value = try lookup(chars[index]) + lookup(chars[index + 1]) * 45
      guard value <= 0xFF else { throw DecodingError.valueOutOfRange }
      out.append(UInt8(value))
    }
    return out
  }

  private static func lookup(_ char: UInt8) throws -> Int {
    let value = decodeTable[Int(char)]
    guard value >= 0 else { throw DecodingError.invalidCharacter }
    return value
  }
}
