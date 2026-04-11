import Foundation

actor OrionClient {
    let host: String
    let token: String
    private let session: URLSession
    private let baseURL: String

    init(host: String, token: String) {
        self.host = host
        self.token = token
        self.baseURL = "http://\(host)"
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 10
        self.session = URLSession(configuration: config)
    }

    func getProjects() async throws -> [String] { try await get("/api/projects") }
    func getProjectInfo(root: String) async throws -> ProjectInfo { try await get("/api/projects/info", query: ["root": root]) }
    func getWorkspaces(root: String) async throws -> [Workspace] { try await get("/api/workspaces", query: ["root": root]) }
    func getSessions(repo: String, workspacePaths: [String]) async throws -> [SessionInfo] {
        try await get("/api/sessions", query: ["repo": repo, "workspaces": workspacePaths.joined(separator: ",")])
    }
    func createTerminal(tmuxSession: String) async throws -> CreateTerminalResponse { try await post("/api/terminal", body: ["tmuxSession": tmuxSession]) }
    func launchShell(repoRoot: String, workspacePath: String) async throws -> LaunchShellResponse { try await post("/api/shell", body: ["repoRoot": repoRoot, "workspacePath": workspacePath]) }

    private func get<T: Decodable>(_ path: String, query: [String: String] = [:]) async throws -> T {
        var components = URLComponents(string: "\(baseURL)\(path)")!
        if !query.isEmpty { components.queryItems = query.map { URLQueryItem(name: $0.key, value: $0.value) } }
        var request = URLRequest(url: components.url!)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let (data, response) = try await session.data(for: request)
        try checkResponse(response, data: data)
        return try JSONDecoder().decode(T.self, from: data)
    }

    private func post<T: Decodable>(_ path: String, body: [String: String]) async throws -> T {
        var request = URLRequest(url: URL(string: "\(baseURL)\(path)")!)
        request.httpMethod = "POST"
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(body)
        let (data, response) = try await session.data(for: request)
        try checkResponse(response, data: data)
        return try JSONDecoder().decode(T.self, from: data)
    }

    private func checkResponse(_ response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
            let message = String(data: data, encoding: .utf8) ?? "Unknown error"
            throw OrionError.httpError(statusCode: (response as? HTTPURLResponse)?.statusCode ?? 0, message: message)
        }
    }
}

enum OrionError: LocalizedError {
    case invalidResponse
    case httpError(statusCode: Int, message: String)
    var errorDescription: String? {
        switch self {
        case .invalidResponse: return "Invalid server response"
        case .httpError(let code, let message): return "Server error (\(code)): \(message)"
        }
    }
}
