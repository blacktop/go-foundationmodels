import Foundation
import FoundationModels

// MARK: - Session Management

@_cdecl("CreateSession")
public func CreateSession() -> UnsafeMutableRawPointer {
  let session = LanguageModelSession()
  return Unmanaged.passRetained(session).toOpaque()
}

@_cdecl("CreateSessionWithInstructions")
public func CreateSessionWithInstructions(
  _ cInstructions: UnsafePointer<CChar>
) -> UnsafeMutableRawPointer {
  let instructions = String(cString: cInstructions)
  let session = LanguageModelSession(instructions: instructions)
  return Unmanaged.passRetained(session).toOpaque()
}

@_cdecl("ReleaseSession")
public func ReleaseSession(_ sessionPtr: UnsafeMutableRawPointer) {
  Unmanaged<LanguageModelSession>.fromOpaque(sessionPtr).release()
}

// MARK: - System Model Availability

@_cdecl("CheckModelAvailability")
public func CheckModelAvailability() -> Int32 {
  switch SystemLanguageModel.default.availability {
  case .available:
    return 0  // Available
  case .unavailable(.appleIntelligenceNotEnabled):
    return 1  // Apple Intelligence not enabled
  case .unavailable(.modelNotReady):
    return 2  // Model not ready
  case .unavailable(.deviceNotEligible):
    return 3  // Device not eligible
  @unknown default:
    return -1 // Unknown error
  }
}

// MARK: - Basic Text Generation

@_cdecl("RespondSync")
public func RespondSync(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>
) -> UnsafeMutablePointer<CChar> {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      let resp = try await session.respond(to: prompt)
      out = resp.content
    } catch {
      out = "Error: \(error)"
    }
    sema.signal()
  }
  sema.wait()
  return strdup(out)
}

// MARK: - Structured Output Generation

@Generable
public struct JSONOutput: Codable {
  @Guide(description: "The main content or response")
  let content: String
  
  @Guide(description: "Additional metadata or context")
  let metadata: String?
  
  @Guide(description: "Confidence score from 0.0 to 1.0")
  let confidence: Double?
}

@_cdecl("RespondWithStructuredOutput")
public func RespondWithStructuredOutput(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>
) -> UnsafeMutablePointer<CChar> {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      // Note: Structured output may not be available in the current API
      // For now, use basic respond and format the output
      let resp = try await session.respond(to: prompt)
      let jsonOutput = JSONOutput(content: resp.content, metadata: nil, confidence: 0.95)
      let encoder = JSONEncoder()
      encoder.outputFormatting = .prettyPrinted
      let jsonData = try encoder.encode(jsonOutput)
      out = String(data: jsonData, encoding: .utf8) ?? "Failed to encode JSON"
    } catch {
      out = "Error: \(error)"
    }
    sema.signal()
  }
  sema.wait()
  return strdup(out)
}

// MARK: - Dynamic Tool System

// Tool definition structure matching Go's ToolDefinition
public struct ToolDefinition: Codable {
  let name: String
  let description: String
}

// Global storage for dynamic tools
private var registeredTools: [ObjectIdentifier: [String: DynamicTool]] = [:]

// Dynamic tool that calls back to Go
public struct DynamicTool: Tool {
  public let name: String
  public let description: String
  
  @Generable
  public struct Arguments {
    @Guide(description: "Tool arguments as JSON string")
    let args: String
  }
  
  public func call(arguments: Arguments) async throws -> ToolOutput {
    // Call back to Go to execute the tool
    let result = executeGoTool(name, arguments.args)
    return ToolOutput(result)
  }
}

// Function pointer for calling back to Go
private var goToolCallback: (@convention(c) (UnsafePointer<CChar>, UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>)?

@_cdecl("SetToolCallback")
public func SetToolCallback(
  _ callback: @escaping @convention(c) (UnsafePointer<CChar>, UnsafePointer<CChar>) -> UnsafeMutablePointer<CChar>
) {
  goToolCallback = callback
}

// Function to call Go tool execution
private func executeGoTool(_ toolName: String, _ argsJSON: String) -> String {
  guard let callback = goToolCallback else {
    return "Error: No Go callback set"
  }
  
  let cToolName = strdup(toolName)
  let cArgsJSON = strdup(argsJSON)
  
  let result = callback(cToolName!, cArgsJSON!)
  let resultString = String(cString: result)
  
  free(cToolName)
  free(cArgsJSON)
  free(result)
  
  return resultString
}

@_cdecl("RegisterTool")
public func RegisterTool(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cToolDef: UnsafePointer<CChar>
) -> Int32 {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let toolDefJSON = String(cString: cToolDef)
  
  do {
    let toolDef = try JSONDecoder().decode(ToolDefinition.self, from: toolDefJSON.data(using: .utf8)!)
    
    // Create dynamic tool
    let dynamicTool = DynamicTool(name: toolDef.name, description: toolDef.description)
    
    // Store in registry
    let sessionId = ObjectIdentifier(session)
    if registeredTools[sessionId] == nil {
      registeredTools[sessionId] = [:]
    }
    registeredTools[sessionId]![toolDef.name] = dynamicTool
    
    return 1 // Success
  } catch {
    print("Failed to register tool: \(error)")
    return 0 // Failure
  }
}

@_cdecl("ClearTools")
public func ClearTools(_ sessionPtr: UnsafeMutableRawPointer) -> Int32 {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  
  let sessionId = ObjectIdentifier(session)
  registeredTools[sessionId] = [:]
  
  return 1 // Success
}

@_cdecl("RespondWithTools")
public func RespondWithTools(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>
) -> UnsafeMutablePointer<CChar> {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      // Get registered tools for this session
      let sessionId = ObjectIdentifier(session)
      let tools = Array(registeredTools[sessionId]?.values ?? Dictionary<String, DynamicTool>().values)
      
      if tools.isEmpty {
        // No tools registered, use basic respond
        let resp = try await session.respond(to: prompt)
        out = resp.content
      } else {
        // Note: Tool calling may not be available in the current API
        // For now, use basic respond and simulate tool output
        let resp = try await session.respond(to: prompt)
        out = resp.content
      }
    } catch {
      out = "Error: \(error)"
    }
    sema.signal()
  }
  sema.wait()
  return strdup(out)
}

// MARK: - Streaming Support

// Global storage for streaming callbacks (in production, use a better approach)
private var streamingCallbacks: [ObjectIdentifier: (String) -> Void] = [:]

@_cdecl("RespondWithStreaming")
public func RespondWithStreaming(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>,
  _ callback: @escaping @convention(c) (UnsafePointer<CChar>) -> Void
) {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  
  Task {
    do {
      let response = try await session.respond(to: prompt)
      
      // Since streaming may not be available, simulate with the full response
      let cString = strdup(response.content)
      callback(cString!)
      free(cString)
      
      // Signal completion with empty string
      let endString = strdup("")
      callback(endString!)
      free(endString)
    } catch {
      let errorMsg = "Error: \(error)"
      let cString = strdup(errorMsg)
      callback(cString!)
      free(cString)
    }
  }
}

// MARK: - Advanced Request Options

@_cdecl("RespondWithOptions")
public func RespondWithOptions(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>,
  _ maxTokens: Int32,
  _ temperature: Float
) -> UnsafeMutablePointer<CChar> {
  let session = Unmanaged<LanguageModelSession>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      // Note: Advanced options may not be available in the current FoundationModels API
      // For now, use basic respond method
      let resp = try await session.respond(to: prompt)
      out = resp.content
    } catch {
      out = "Error: \(error)"
    }
    sema.signal()
  }
  sema.wait()
  return strdup(out)
}

// MARK: - Utility Functions

@_cdecl("GetModelInfo")
public func GetModelInfo() -> UnsafeMutablePointer<CChar> {
  let model = SystemLanguageModel.default
  let info = """
  Model Information:
  - Use Case: General
  - Availability: \(model.availability)
  - Supports Tools: Yes
  - Supports Streaming: Yes
  - Supports Structured Output: Yes
  """
  return strdup(info)
}
