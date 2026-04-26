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
    func createWorkspace(root: String, name: String, baseRef: String) async throws -> Workspace {
        try await post("/api/workspaces", body: ["root": root, "name": name, "baseRef": baseRef])
    }
    func getSessions(repo: String, workspacePaths: [String]) async throws -> [SessionInfo] {
        try await get("/api/sessions", query: ["repo": repo, "workspaces": workspacePaths.joined(separator: ",")])
    }
    func createTerminal(tmuxSession: String) async throws -> CreateTerminalResponse { try await post("/api/terminal", body: ["tmuxSession": tmuxSession]) }
    func launchShell(repoRoot: String, workspacePath: String) async throws -> LaunchShellResponse { try await post("/api/shell", body: ["repoRoot": repoRoot, "workspacePath": workspacePath]) }
    func convertChatToTerminal(repoRoot: String, workspacePath: String, sessionId: String, chatKind: String, model: String? = nil, reasoningEffort: String? = nil, permissionMode: String? = nil, collaborationMode: String? = nil) async throws -> LaunchAgentResponse {
        var body = ["repoRoot": repoRoot, "workspacePath": workspacePath, "sessionId": sessionId, "chatKind": chatKind]
        if let model, !model.isEmpty { body["model"] = model }
        if let reasoningEffort, !reasoningEffort.isEmpty { body["reasoningEffort"] = reasoningEffort }
        if let permissionMode, !permissionMode.isEmpty { body["permissionMode"] = permissionMode }
        if let collaborationMode, !collaborationMode.isEmpty { body["collaborationMode"] = collaborationMode }
        return try await post("/api/convert-chat-to-terminal", body: body)
    }

    // Agents
    func getAgentTypes(root: String) async throws -> [AgentType] { try await get("/api/agents", query: ["root": root]) }
    func launchAgent(repoRoot: String, workspacePath: String, agentType: String) async throws -> LaunchAgentResponse {
        try await post("/api/agent", body: ["repoRoot": repoRoot, "workspacePath": workspacePath, "agentType": agentType])
    }

    func launchCodexChat(repoRoot: String, workspacePath: String, threadId: String? = nil, tmuxSession: String? = nil, options: CodexLaunchOptions? = CodexLaunchOptions()) async throws -> LaunchCodexChatResponse {
        var body = ["repoRoot": repoRoot, "workspacePath": workspacePath]
        if let threadId, !threadId.isEmpty { body["threadId"] = threadId }
        if let tmuxSession, !tmuxSession.isEmpty { body["tmuxSession"] = tmuxSession }
        if let options {
            if !options.model.isEmpty { body["model"] = options.model }
            body["reasoningEffort"] = options.reasoningEffort
            body["approvalPolicy"] = options.approvalPolicy
            body["sandboxMode"] = options.sandboxMode
            body["collaborationMode"] = options.collaborationMode
        }
        return try await post("/api/codex-chat", body: body)
    }

    func getCodexHistory(workspacePath: String, limit: Int = 20) async throws -> [CodexHistoryThread] {
        try await get("/api/codex-chat/history", query: ["workspace": workspacePath, "limit": String(limit)])
    }

    func launchClaudeChat(repoRoot: String, workspacePath: String, threadId: String? = nil, tmuxSession: String? = nil, options: ClaudeLaunchOptions? = nil) async throws -> LaunchClaudeChatResponse {
        var body = ["repoRoot": repoRoot, "workspacePath": workspacePath]
        if let threadId, !threadId.isEmpty { body["threadId"] = threadId }
        if let tmuxSession, !tmuxSession.isEmpty { body["tmuxSession"] = tmuxSession }
        if let model = options?.model, !model.isEmpty { body["model"] = model }
        if let reasoningEffort = options?.reasoningEffort, !reasoningEffort.isEmpty { body["reasoningEffort"] = reasoningEffort }
        if let approvalPolicy = options?.approvalPolicy, !approvalPolicy.isEmpty { body["approvalPolicy"] = approvalPolicy }
        if let sandboxMode = options?.sandboxMode, !sandboxMode.isEmpty { body["sandboxMode"] = sandboxMode }
        if let permissionMode = options?.permissionMode, !permissionMode.isEmpty { body["permissionMode"] = permissionMode }
        return try await post("/api/claude-chat", body: body)
    }

    func sendCodexChatMessage(sessionId: String, text: String) async throws {
        let _: [String: String] = try await post("/api/codex-chat/message", body: ["sessionId": sessionId, "text": text])
    }

    func sendClaudeChatMessage(sessionId: String, text: String) async throws {
        let _: [String: String] = try await post("/api/claude-chat/message", body: ["sessionId": sessionId, "text": text])
    }

    func getChangedFiles(workspacePath: String, base: String = "") async throws -> [GitChangedFile] {
        try await get("/api/git/changes", query: ["workspace": workspacePath, "base": base])
    }

    func getUnifiedDiff(workspacePath: String, base: String = "", filePath: String) async throws -> String {
        let response: GitDiffResponse = try await get("/api/git/diff", query: ["workspace": workspacePath, "base": base, "file": filePath])
        return response.diff
    }

    // Server management
    func getServerStatuses(root: String, workspace: String) async throws -> [ServerStatus] {
        try await get("/api/servers", query: ["root": root, "workspace": workspace])
    }
    func startServers(repoRoot: String, workspacePath: String, isMain: Bool) async throws -> [ServerStatus] {
        try await postJSON("/api/servers/start", body: StartServersRequest(repoRoot: repoRoot, workspacePath: workspacePath, isMain: isMain))
    }
    func stopServers(workspacePath: String) async throws {
        let _: [String: String] = try await post("/api/servers/stop", body: ["workspacePath": workspacePath])
    }

    // Config
    func getConfig() async throws -> AppConfig { try await get("/api/config") }

    // Kill a tmux session
    func killSession(tmuxSession: String) async throws {
        let _: [String: String] = try await post("/api/kill-session", body: ["tmuxSession": tmuxSession])
    }

    // MARK: - HTTP Helpers

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

    private func postJSON<B: Encodable, T: Decodable>(_ path: String, body: B) async throws -> T {
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
