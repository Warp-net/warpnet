// Modified by Warpnet. See NOTICE.

import Combine
import Foundation
import Models
import Observation
import os
import SwiftUI
import OSLog
import WarpnetTransport

@Observable public final class Client: Equatable, Identifiable, Hashable, @unchecked Sendable {
  public static func == (lhs: Client, rhs: Client) -> Bool {
    lhs.id == rhs.id
  }

  public enum Version: String, Sendable {
    case v1, v2
  }

  public enum ClientError: Error {
    case unexpectedRequest
  }

  public var id: String {
    "\(isAuth)\(server)"
  }

  public func hash(into hasher: inout Hasher) {
    hasher.combine(id)
  }

  public let server: String
  public let version: Version
  public let session: WarpnetSession?
  private let urlSession: URLSession
  private let decoder = JSONDecoder()
  
  private let logger = Logger(subsystem: "site.warpnet.warpfone", category: "networking")

  // Putting all mutable state inside an `OSAllocatedUnfairLock` makes `Client`
  // provably `Sendable`. The lock is a struct, but it uses a `ManagedBuffer`
  // reference type to hold its associated state.
  private let critical: OSAllocatedUnfairLock<Critical>
  private struct Critical: Sendable {
    var connections: Set<String> = []
  }

  public var isAuth: Bool {
    session != nil
  }

  public var connections: Set<String> {
    critical.withLock { $0.connections }
  }

  public init(server: String, version: Version = .v1, session: WarpnetSession? = nil) {
    self.server = server
    self.version = version
    self.session = session
    critical = .init(initialState: Critical(connections: [server]))
    urlSession = URLSession.shared
    decoder.keyDecodingStrategy = .convertFromSnakeCase
  }

  public func addConnections(_ connections: [String]) {
    critical.withLock {
      $0.connections.formUnion(connections)
    }
  }

  public func hasConnection(with url: URL) -> Bool {
    guard let host = url.host else { return false }
    return critical.withLock {
      if let rootHost = host.split(separator: ".", maxSplits: 1).last {
        // Sometimes the connection is with the root host instead of a subdomain
        // eg. Mastodon runs on mastdon.domain.com but the connection is with domain.com
        $0.connections.contains(host) || $0.connections.contains(String(rootHost))
      } else {
        $0.connections.contains(host)
      }
    }
  }

  private func makeURL(scheme: String = "https",
                       endpoint: Endpoint,
                       forceVersion: Version? = nil,
                       forceServer: String? = nil) throws -> URL
  {
    var components = URLComponents()
    components.scheme = scheme
    components.host = forceServer ?? server
    components.path += "/api/\(forceVersion?.rawValue ?? version.rawValue)/\(endpoint.path())"
    components.queryItems = endpoint.queryItems()
    guard let url = components.url else {
      throw ClientError.unexpectedRequest
    }
    return url
  }

  private func makeURLRequest(url: URL, endpoint: Endpoint, httpMethod: String) -> URLRequest {
    var request = URLRequest(url: url)
    request.httpMethod = httpMethod
    if let json = endpoint.jsonValue {
      let encoder = JSONEncoder()
      encoder.keyEncodingStrategy = .convertToSnakeCase
      encoder.outputFormatting = .sortedKeys
      do {
        let jsonData = try encoder.encode(json)
        request.httpBody = jsonData
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
      } catch {
        logger.error("Error encoding JSON: \(error.localizedDescription)")
      }
    }
    return request
  }

  private func makeGet(endpoint: Endpoint) throws -> URLRequest {
    let url = try makeURL(endpoint: endpoint)
    return makeURLRequest(url: url, endpoint: endpoint, httpMethod: "GET")
  }

  public func get<Entity: Decodable>(endpoint: Endpoint, forceVersion: Version? = nil) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: "GET", forceVersion: forceVersion)
  }

  public func getWithLink<Entity: Decodable>(endpoint: Endpoint) async throws -> (Entity, LinkHandler?) {
    let request = try makeGet(endpoint: endpoint)
    let (data, httpResponse) = try await urlSession.data(for: request)
    var linkHandler: LinkHandler?
    if let response = httpResponse as? HTTPURLResponse,
       let link = response.allHeaderFields["Link"] as? String
    {
      linkHandler = .init(rawLink: link)
    }
    logResponseOnError(httpResponse: httpResponse, data: data)
    logger.log(level: .info, "\(request)")
    return try (decoder.decode(Entity.self, from: data), linkHandler)
  }

  public func post<Entity: Decodable>(endpoint: Endpoint, forceVersion: Version? = nil) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: "POST", forceVersion: forceVersion)
  }

  public func post(endpoint: Endpoint, forceVersion: Version? = nil) async throws -> HTTPURLResponse? {
    let url = try makeURL(endpoint: endpoint, forceVersion: forceVersion)
    let request = makeURLRequest(url: url, endpoint: endpoint, httpMethod: "POST")
    let (_, httpResponse) = try await urlSession.data(for: request)
    return httpResponse as? HTTPURLResponse
  }

  public func patch(endpoint: Endpoint) async throws -> HTTPURLResponse? {
    let url = try makeURL(endpoint: endpoint)
    let request = makeURLRequest(url: url, endpoint: endpoint, httpMethod: "PATCH")
    let (_, httpResponse) = try await urlSession.data(for: request)
    return httpResponse as? HTTPURLResponse
  }

  public func put<Entity: Decodable>(endpoint: Endpoint, forceVersion: Version? = nil) async throws -> Entity {
    try await makeEntityRequest(endpoint: endpoint, method: "PUT", forceVersion: forceVersion)
  }

  public func delete(endpoint: Endpoint, forceVersion: Version? = nil) async throws -> HTTPURLResponse? {
    let url = try makeURL(endpoint: endpoint, forceVersion: forceVersion)
    let request = makeURLRequest(url: url, endpoint: endpoint, httpMethod: "DELETE")
    let (_, httpResponse) = try await urlSession.data(for: request)
    return httpResponse as? HTTPURLResponse
  }

  private func makeEntityRequest<Entity: Decodable>(endpoint: Endpoint,
                                                    method: String,
                                                    forceVersion: Version? = nil) async throws -> Entity
  {
    let url = try makeURL(endpoint: endpoint, forceVersion: forceVersion)
    let request = makeURLRequest(url: url, endpoint: endpoint, httpMethod: method)
    let (data, httpResponse) = try await urlSession.data(for: request)
    logger.log(level: .info, "\(request)")
    logResponseOnError(httpResponse: httpResponse, data: data)
    do {
      return try decoder.decode(Entity.self, from: data)
    } catch {
      if var serverError = try? decoder.decode(ServerError.self, from: data) {
        if let httpResponse = httpResponse as? HTTPURLResponse {
          serverError.httpCode = httpResponse.statusCode
        }
        throw serverError
      }
      throw error
    }
  }

  public func makeWebSocketTask(endpoint: Endpoint, instanceStreamingURL: URL?) throws -> URLSessionWebSocketTask {
    let url = try makeURL(scheme: "wss", endpoint: endpoint, forceServer: instanceStreamingURL?.host)
    return urlSession.webSocketTask(with: url)
  }

  public func mediaUpload<Entity: Decodable>(endpoint: Endpoint,
                                             version: Version,
                                             method: String,
                                             mimeType: String,
                                             filename: String,
                                             data: Data) async throws -> Entity
  {
    let request = try makeFormDataRequest(endpoint: endpoint,
                                          version: version,
                                          method: method,
                                          mimeType: mimeType,
                                          filename: filename,
                                          data: data)
    let (data, httpResponse) = try await urlSession.data(for: request)
    logResponseOnError(httpResponse: httpResponse, data: data)
    do {
      return try decoder.decode(Entity.self, from: data)
    } catch {
      if let serverError = try? decoder.decode(ServerError.self, from: data) {
        throw serverError
      }
      throw error
    }
  }
  
  public func mediaUpload(endpoint: Endpoint,
                          version: Version,
                          method: String,
                          mimeType: String,
                          filename: String,
                          data: Data) async throws -> HTTPURLResponse?
  {
    let request = try makeFormDataRequest(endpoint: endpoint,
                                          version: version,
                                          method: method,
                                          mimeType: mimeType,
                                          filename: filename,
                                          data: data)
    let (_, httpResponse) = try await urlSession.data(for: request)
    return httpResponse as? HTTPURLResponse
  }
  
  private func makeFormDataRequest(endpoint: Endpoint,
                                   version: Version,
                                   method: String,
                                   mimeType: String,
                                   filename: String,
                                   data: Data) throws -> URLRequest {
    let url = try makeURL(endpoint: endpoint, forceVersion: version)
    var request = makeURLRequest(url: url, endpoint: endpoint, httpMethod: method)
    let boundary = UUID().uuidString
    request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
    let httpBody = NSMutableData()
    httpBody.append("--\(boundary)\r\n".data(using: .utf8)!)
    httpBody.append("Content-Disposition: form-data; name=\"\(filename)\"; filename=\"\(filename)\"\r\n".data(using: .utf8)!)
    httpBody.append("Content-Type: \(mimeType)\r\n".data(using: .utf8)!)
    httpBody.append("\r\n".data(using: .utf8)!)
    httpBody.append(data)
    httpBody.append("\r\n--\(boundary)--\r\n".data(using: .utf8)!)
    request.httpBody = httpBody as Data
    return request
  }

  private func logResponseOnError(httpResponse: URLResponse, data: Data) {
    if let httpResponse = httpResponse as? HTTPURLResponse, httpResponse.statusCode > 299 {
      let error = "HTTP Response error: \(httpResponse.statusCode), response: \(httpResponse), data: \(String(data: data, encoding: .utf8) ?? "")"
      logger.error("\(error)")
    }
  }
}
