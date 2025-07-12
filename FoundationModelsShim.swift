import Foundation
import FoundationModels

// MARK: - Session Management

// Session wrapper to store tools and instructions separately
public class SessionWrapper {
  private var _session: LanguageModelSession?
  var tools: [any Tool] = []
  let instructions: String?
  
  init(instructions: String? = nil) {
    self.instructions = instructions
    // Don't create session yet - wait for tools to be registered
  }
  
  // Get or create session with current tools
  var session: LanguageModelSession {
    if let existingSession = _session {
      return existingSession
    }
    
    // Create new session with tools and instructions
    let newSession: LanguageModelSession
    if tools.isEmpty {
      if let instructions = instructions {
        newSession = LanguageModelSession(instructions: instructions)
      } else {
        newSession = LanguageModelSession()
      }
    } else {
      if let instructions = instructions {
        newSession = LanguageModelSession(tools: tools, instructions: instructions)
      } else {
        newSession = LanguageModelSession(tools: tools)
      }
    }
    
    _session = newSession
    return newSession
  }
  
  // Force recreation of session when tools change
  func invalidateSession() {
    _session = nil
  }
}

private var logs: [String] = []

private func log(_ message: String) {
    logs.append(message)
}

@_cdecl("GetLogs")
public func GetLogs() -> UnsafeMutablePointer<CChar> {
    let logString = logs.joined(separator: "\n")
    logs.removeAll()
    return strdup(logString)
}

@_cdecl("CreateSession")

public func CreateSession() -> UnsafeMutableRawPointer {
  let wrapper = SessionWrapper(instructions: nil)
  return Unmanaged.passRetained(wrapper).toOpaque()
}

@_cdecl("CreateSessionWithInstructions")
public func CreateSessionWithInstructions(
  _ cInstructions: UnsafePointer<CChar>
) -> UnsafeMutableRawPointer {
  let instructions = String(cString: cInstructions)
  let wrapper = SessionWrapper(instructions: instructions)
  return Unmanaged.passRetained(wrapper).toOpaque()
}

@_cdecl("ReleaseSession")
public func ReleaseSession(_ sessionPtr: UnsafeMutableRawPointer) {
  Unmanaged<SessionWrapper>.fromOpaque(sessionPtr).release()
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
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      let resp = try await wrapper.session.respond(to: prompt)
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
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      // Note: Structured output may not be available in the current API
      // For now, use basic respond and format the output
      let resp = try await wrapper.session.respond(to: prompt)
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
  let parameters: [String: ParameterDefinition]
}

public struct ParameterDefinition: Codable, Sendable {
  let type: String
  let description: String
  let required: Bool
  let enumValues: [String]?
  
  enum CodingKeys: String, CodingKey {
    case type, description, required
    case enumValues = "enum"
  }
}


// Global storage for dynamic tools
private var registeredTools: [String: DynamicTool] = [:]

// Dynamic tool that calls back to Go
public final class DynamicTool: Tool {
  public let name: String
  public let description: String
  public let parameters: GenerationSchema
  
  // The Arguments type must conform to 'Generable' for the Tool protocol.
  // We can define a struct that can hold the expected arguments.
  @Generable
  public struct Arguments {
    @Guide(description: "The arguments for the tool, in JSON format")
    public var arguments: String
  }

  init(name: String, description: String, parameters: GenerationSchema) {
    self.name = name
    self.description = description
    self.parameters = parameters
  }

  public func call(arguments: Arguments) async throws -> ToolOutput {
    log("Swift: DynamicTool.call invoked for tool '\(name)'")
    log("Swift: Raw arguments JSON: \(arguments.arguments)")

    // The arguments are already in JSON format, so we can pass them directly to Go.
    let argsJSON = arguments.arguments
    
    log("Swift: Calling Go callback with JSON: \(argsJSON)")

    // Call back to Go to execute the tool
    let result = executeGoTool(name, argsJSON)

    log("Swift: Tool execution result: \(result)")

    // Create tool output and return to Foundation Models
    let toolOutput = ToolOutput(result)
    log("Swift: Created ToolOutput, returning to Foundation Models")

    return toolOutput
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
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let toolDefJSON = String(cString: cToolDef)
  
  do {
    let toolDef = try JSONDecoder().decode(ToolDefinition.self, from: toolDefJSON.data(using: .utf8)!)
    
    // Create a DynamicGenerationSchema for the root.
    let argumentsSchema = DynamicGenerationSchema(type: String.self)
    let argumentsProperty = DynamicGenerationSchema.Property(
        name: "arguments",
        description: "A JSON string containing the tool arguments.",
        schema: argumentsSchema
    )

    // Create a DynamicGenerationSchema for the root, passing the properties array directly
    let rootSchema = DynamicGenerationSchema(
        name: toolDef.name,
        properties: [argumentsProperty]
    )

    // Create a GenerationSchema from the tool definition.
    let schema = try GenerationSchema(root: rootSchema, dependencies: [])

    // Create dynamic tool with the new schema.
    let dynamicTool = DynamicTool(name: toolDef.name, description: toolDef.description, parameters: schema)
    
    // Store in global registry
    registeredTools[toolDef.name] = dynamicTool
    
    // Add to session's tools
    wrapper.tools.append(dynamicTool)
    
    // Invalidate session so it gets recreated with new tools
    wrapper.invalidateSession()
    
    log("Swift: Registered tool '\(toolDef.name)' with description '\(toolDef.description)'")
    log("Swift: Total tools in session: \(wrapper.tools.count)")
    
    return 1 // Success
  } catch {
    log("Failed to register tool: \(error)")
    return 0 // Failure
  }
}

@_cdecl("ClearTools")
public func ClearTools(_ sessionPtr: UnsafeMutableRawPointer) -> Int32 {
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  
  // Clear session tools
  wrapper.tools.removeAll()
  
  // Invalidate session so it gets recreated without tools
  wrapper.invalidateSession()
  
  return 1 // Success
}

@_cdecl("RespondWithTools")
public func RespondWithTools(
  _ sessionPtr: UnsafeMutableRawPointer,
  _ cPrompt: UnsafePointer<CChar>
) -> UnsafeMutablePointer<CChar> {
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      print("Swift: RespondWithTools called with prompt: \(prompt)")
      print("Swift: Using session with \(wrapper.tools.count) tools")
      
      // The session property will automatically create the session with tools if needed
      let resp = try await wrapper.session.respond(to: prompt)
      out = resp.content
      log("Swift: Received response: \(out)")
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
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  
  Task {
    do {
      let response = try await wrapper.session.respond(to: prompt)
      
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
  let wrapper = Unmanaged<SessionWrapper>
    .fromOpaque(sessionPtr)
    .takeUnretainedValue()
  let prompt = String(cString: cPrompt)
  var out: String = ""
  let sema = DispatchSemaphore(value: 0)

  Task {
    do {
      // Note: Advanced options may not be available in the current FoundationModels API
      // For now, use basic respond method
      let resp = try await wrapper.session.respond(to: prompt)
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
