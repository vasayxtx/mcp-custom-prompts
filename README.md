# MCP Prompt Engine

A simple Model Control Protocol (MCP) server for managing and serving dynamic prompt templates using Go's `text/template` engine.
It allows you to create reusable prompt templates with variable placeholders, conditionals, loops, and partials that can be filled in at runtime.

## Features

- Go `text/template` syntax with variables, conditionals, loops, and partials
- Automatic JSON argument parsing with string fallback
- Environment variable injection and built-in functions
- Efficient file watching with hot-reload capabilities using fsnotify
- Compatible with Claude Desktop, Claude Code, and other MCP clients

## Installation

```bash
go install github.com/vasayxtx/mcp-prompt-engine@latest
```

### Building from source

```bash
make build
```

## Usage

### Creating Prompt Templates

Create a directory to store your prompt templates. Each template should be a `.tmpl` file using Go's `text/template` syntax with the following format:

```go
{{/* Brief description of the prompt */}}
Your prompt text here with {{.template_variable}} placeholders.
```

The first line comment (`{{/* description */}}`) is used as the prompt description, and the rest of the file is the prompt template.

### Template Syntax

The server uses Go's `text/template` engine, which provides powerful templating capabilities:

- **Variables**: `{{.variable_name}}` - Access template variables
- **Built-in variables**: 
  - `{{.date}}` - Current date and time
- **Conditionals**: `{{if .condition}}...{{end}}`, `{{if .condition}}...{{else}}...{{end}}`
- **Logical operators**: `{{if and .condition1 .condition2}}...{{end}}`, `{{if or .condition1 .condition2}}...{{end}}`
- **Loops**: `{{range .items}}...{{end}}`
- **Template inclusion**: `{{template "partial_name" .}}` or `{{template "partial_name" dict "key" "value"}}`

### JSON Argument Parsing

The server automatically parses argument values as JSON when possible, enabling rich data types in templates:

- **Booleans**: `true`, `false` → Go boolean values
- **Numbers**: `42`, `3.14` → Go numeric values  
- **Arrays**: `["item1", "item2"]` → Go slices for use with `{{range}}`
- **Objects**: `{"key": "value"}` → Go maps for structured data
- **Strings**: Invalid JSON falls back to string values

This allows for advanced template operations like:
```go
{{range .items}}Item: {{.}}{{end}}
{{if .enabled}}Feature is enabled{{end}}
{{.config.timeout}} seconds
```

To disable JSON parsing and treat all arguments as strings, use the `--disable-json-args` flag.

### Partials (Reusable Components)

Create reusable template components by prefixing filenames with `_`. These partials can be included in other templates using the `{{template "partial_name" .}}` syntax. The system automatically detects which partials are used by each template:

**Example partial** (`_header.tmpl`):
```go
{{/* Common header partial */}}
You are an experienced {{.role}} tasked with {{.task}}.
Current date: {{.date}}
{{if .context}}Context: {{.context}}{{end}}
```

**Using partials in main templates**:
```go
{{/* Main prompt using header partial */}}
{{template "_header" dict "role" "software developer" "task" "code review" "context" .context}}

Please review the following code:
{{.code}}
```

### Built-in Functions

The server provides these built-in template functions:

- `dict` - Create a map from key-value pairs: `{{template "partial" dict "key1" "value1" "key2" "value2"}}`

### Example Prompt Template

Here's a complete example of a code review prompt template (`code_review.tmpl`):

```go
{{/* Perform a code review. Optionally include urgency. Args: language, project_root, src_path, [urgency_level], [context] */}}
{{template "_header" dict "role" "software developer" "task" "performing a thorough code review" "date" .date "context" .context}}

Here are the details of the code you need to review:

Programming Language:
<programming_language>
{{.language}}
</programming_language>

Project Root Directory:
<project_root>
{{.project_root}}
</project_root>

File or Directory for Review:
<review_path>
{{.src_path}}
</review_path>

{{if .urgency_level}}
Urgency: Please address this review with {{.urgency_level}} urgency.
{{end}}

Please conduct a comprehensive code review focusing on the following aspects:
1. Code quality
2. Adherence to best practices
3. Potential bugs or logical errors
4. Performance optimization opportunities
5. Security vulnerabilities or concerns

{{template "_analysis_footer" dict "analysis_type" "review"}}

Remember to be specific in your recommendations, providing clear guidance on how to improve the code.
```

### Running the Server

```bash
./mcp-prompt-engine -prompts /path/to/prompts/directory -log-file /path/to/log/file
```

### Rendering a Template to Stdout

You can also render a specific template directly to stdout without starting the server:

```bash
./mcp-prompt-engine -prompts /path/to/prompts/directory -template template_name
```

This is useful for testing templates or using them in shell scripts.

Options:
- `-prompts`: Directory containing prompt template files (default: "./prompts")
- `-log-file`: Path to log file (if not specified, logs to stdout)
- `-template`: Template name to render to stdout (bypasses server mode)
- `-disable-json-args`: Disable JSON argument parsing, treat all arguments as strings
- `-version`: Show version and exit

## Configuring Claude Desktop

To use this MCP server with Claude Desktop, add the following configuration to your Claude Desktop settings:

```json
{
  "custom-prompts": {
    "command": "/path/to/mcp-prompt-engine",
    "args": [
      "-prompts",
      "/path/to/directory/with/prompts",
      "-log-file",
      "/path/to/log/file"
    ],
    "env": {
      "CONTEXT": "Default context value",
      "PROJECT_ROOT": "/path/to/project"
    }
  }
}
```

### Environment Variable Injection

The server automatically injects environment variables into your prompts. If an environment variable with the same name as a template variable (in uppercase) is found, it will be used to fill the template.

For example, if your prompt contains `{{.username}}` and you set the environment variable `USERNAME=john`, the server will automatically replace `{{.username}}` with `john` in the prompt.

In the Claude Desktop configuration above, the `"env"` section allows you to define environment variables that will be injected into your prompts.

## How It Works

1. **Server startup**: The server parses all `.tmpl` files on startup:
   - Loads partials (files starting with `_`) for reuse
   - Loads main prompt templates (files not starting with `_`)
   - Extracts template variables by analyzing the template content and its used partials
   - Only partials that are actually referenced by the template are included
   - Template arguments are extracted from patterns like `{{.fieldname}}` and `dict "key" .value`
   - Sets up efficient file watching using fsnotify for hot-reload capabilities

2. **File watching and hot-reload**: The server automatically detects changes:
   - Monitors the prompts directory for file modifications, additions, and removals
   - Automatically reloads templates when changes are detected
   - No server restart required when adding new templates or modifying existing ones

3. **Prompt request processing**: When a prompt is requested:
   - Uses the latest version of templates (automatically reloaded if changed)
   - Prepares template data with built-in variables (like `date`)
   - Merges environment variables and request parameters
   - Executes the template with all data
   - Returns the processed prompt to the client

## License

MIT License - see [LICENSE](./LICENSE) file for details.
