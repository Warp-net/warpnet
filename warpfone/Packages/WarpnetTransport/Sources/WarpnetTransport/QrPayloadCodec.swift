import Foundation

public enum QrPayloadCodec {
  public enum DecodingError: Error, Equatable {
    case tooLong
    case notGzip
    case tooLarge
    case checksumMismatch
    case inflateFailed
    case notUTF8
  }

  static let maxEncodedChars = 8 * 1024
  static let maxDecodedBytes = 64 * 1024

  public static func decode(_ payload: String) throws -> String {
    let trimmed = payload.trimmingCharacters(in: .whitespacesAndNewlines)
    if trimmed.isEmpty || trimmed.hasPrefix("{") { return trimmed }
    guard trimmed.utf8.count <= maxEncodedChars else { throw DecodingError.tooLong }
    let json = try gunzip(Base45.decode(trimmed), limit: maxDecodedBytes)
    guard let text = String(data: json, encoding: .utf8) else { throw DecodingError.notUTF8 }
    return text
  }

  static func gunzip(_ data: Data, limit: Int) throws -> Data {
    let bytes = [UInt8](data)
    guard bytes.count >= 18, bytes[0] == 0x1F, bytes[1] == 0x8B, bytes[2] == 8 else {
      throw DecodingError.notGzip
    }
    let flags = bytes[3]
    var offset = 10
    if flags & 0x04 != 0 {
      guard offset + 2 <= bytes.count else { throw DecodingError.notGzip }
      offset += 2 + (Int(bytes[offset]) | Int(bytes[offset + 1]) << 8)
    }
    for flag: UInt8 in [0x08, 0x10] where flags & flag != 0 {
      while offset < bytes.count, bytes[offset] != 0 {
        offset += 1
      }
      offset += 1
    }
    if flags & 0x02 != 0 {
      offset += 2
    }
    guard offset <= bytes.count - 8 else { throw DecodingError.notGzip }

    let trailer = Array(bytes.suffix(8))
    let expectedCRC = littleEndian(trailer[0 ..< 4])
    let expectedSize = littleEndian(trailer[4 ..< 8])
    guard Int(expectedSize) <= limit else { throw DecodingError.tooLarge }

    let inflated = try inflate(Data(bytes[offset ..< bytes.count - 8]))
    guard inflated.count <= limit else { throw DecodingError.tooLarge }
    guard UInt32(inflated.count) == expectedSize, crc32(inflated) == expectedCRC else {
      throw DecodingError.checksumMismatch
    }
    return inflated
  }

  private static func inflate(_ deflate: Data) throws -> Data {
    #if canImport(Darwin)
      do {
        return try (deflate as NSData).decompressed(using: .zlib) as Data
      } catch {
        throw DecodingError.inflateFailed
      }
    #else
      throw DecodingError.inflateFailed
    #endif
  }

  private static func littleEndian(_ bytes: ArraySlice<UInt8>) -> UInt32 {
    bytes.reversed().reduce(0) { $0 << 8 | UInt32($1) }
  }

  private static let crcTable: [UInt32] = (0 ..< 256).map { index in
    var value = UInt32(index)
    for _ in 0 ..< 8 {
      value = value & 1 == 1 ? 0xEDB8_8320 ^ (value >> 1) : value >> 1
    }
    return value
  }

  static func crc32(_ data: Data) -> UInt32 {
    var crc: UInt32 = 0xFFFF_FFFF
    for byte in data {
      crc = crcTable[Int((crc ^ UInt32(byte)) & 0xFF)] ^ (crc >> 8)
    }
    return crc ^ 0xFFFF_FFFF
  }
}
