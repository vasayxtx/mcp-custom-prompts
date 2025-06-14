package main

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/vasayxtx/mcp-custom-prompts/mcptest"
)

func TestExtractTemplateArguments(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		partials    map[string]string
		expected    []string
		description string
		shouldError bool
	}{
		{
			name:        "empty template",
			content:     "{{/* Empty template */}}\nNo arguments here",
			partials:    map[string]string{},
			expected:    []string{},
			description: "Empty template",
			shouldError: false,
		},
		{
			name:        "single argument",
			content:     "{{/* Single argument template */}}\nHello {{.name}}",
			partials:    map[string]string{},
			expected:    []string{"name"},
			description: "Single argument template",
			shouldError: false,
		},
		{
			name:        "multiple arguments",
			content:     "{{/* Multiple arguments template */}}\nHello {{.name}}, your project is {{.project}} and language is {{.language}}",
			partials:    map[string]string{},
			expected:    []string{"name", "project", "language"},
			description: "Multiple arguments template",
			shouldError: false,
		},
		{
			name:        "arguments with built-in date",
			content:     "{{/* Template with date */}}\nToday is {{.date}} and user is {{.username}}",
			partials:    map[string]string{},
			expected:    []string{"username"}, // date is built-in, should be filtered out
			description: "Template with date",
			shouldError: false,
		},
		{
			name:        "template with used partial only",
			content:     "{{/* Template with used partial only */}}\n{{template \"_header\" dict \"role\" .role \"task\" .task}}\nUser: {{.username}}",
			partials:    map[string]string{"header": "You are {{.role}} doing {{.task}}", "footer": "End with {{.conclusion}}"},
			expected:    []string{"role", "task", "username"}, // should NOT include conclusion from unused footer
			description: "Template with used partial only",
			shouldError: false,
		},
		{
			name:        "template with multiple used partials",
			content:     "{{/* Template with multiple partials */}}\n{{template \"_header\" dict \"role\" .role}}\n{{template \"_footer\" dict \"conclusion\" .conclusion}}\nUser: {{.username}}",
			partials:    map[string]string{"header": "You are {{.role}}", "footer": "End with {{.conclusion}}", "unused": "This has {{.unused_var}}"},
			expected:    []string{"role", "conclusion", "username"}, // should NOT include unused_var
			description: "Template with multiple partials",
			shouldError: false,
		},
		{
			name:        "template with no partials used",
			content:     "{{/* Template with no partials */}}\nJust {{.simple}} content",
			partials:    map[string]string{"header": "You are {{.role}}", "footer": "End with {{.conclusion}}"},
			expected:    []string{"simple"}, // should NOT include role or conclusion
			description: "Template with no partials used",
			shouldError: false,
		},
		{
			name:        "duplicate arguments",
			content:     "{{/* Duplicate arguments */}}\n{{.user}} said hello to {{.user}} again",
			partials:    map[string]string{},
			expected:    []string{"user"},
			description: "Duplicate arguments",
			shouldError: false,
		},
		{
			name:        "argument in if statement",
			content:     "{{/* Template with if statement */}}\\n{{if .show_details}}Details: {{.details_text}}{{end}}\\nAlways show: {{.always_visible}}",
			partials:    map[string]string{},
			expected:    []string{"show_details", "details_text", "always_visible"},
			description: "Template with if statement",
			shouldError: false,
		},
		{
			name:    "cyclic partial references",
			content: "{{/* Template with cyclic partials */}}\n{{template \"_a\" .}}\nMain content: {{.main}}",
			partials: map[string]string{
				"_a": "Partial A with {{.a_var}} {{template \"_b\" .}}",
				"_b": "Partial B with {{.b_var}} {{template \"_c\" .}}",
				"_c": "Partial C with {{.c_var}} {{template \"_a\" .}}", // Creates a cycle: a -> b -> c -> a
			},
			expected:    nil,
			description: "Template with cyclic partials",
			shouldError: true,
		},
		{
			name:    "deeply nested partials",
			content: "{{/* Template with deeply nested partials */}}\n{{template \"_level1\" .}}\nMain content: {{.main_var}}",
			partials: map[string]string{
				"_level1": "Level 1 with {{.level1_var}} {{template \"_level2\" .}}",
				"_level2": "Level 2 with {{.level2_var}} {{template \"_level3\" .}}",
				"_level3": "Level 3 with {{.level3_var}} {{template \"_level4\" .}}",
				"_level4": "Level 4 with {{.level4_var}}",
				"_unused": "This partial is not used {{.unused_var}}",
			},
			expected:    []string{"level1_var", "level2_var", "level3_var", "level4_var", "main_var"},
			description: "Template with deeply nested partials",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tempDir, tt.name+".tmpl")
			err := os.WriteFile(testFile, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			got, err := extractPromptArguments(testFile, tt.partials)

			if tt.shouldError {
				if err == nil {
					t.Errorf("extractPromptArguments() expected error, but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("extractPromptArguments() error = %v", err)
			}

			// Sort both slices for consistent comparison
			sort.Strings(got)
			sort.Strings(tt.expected)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("extractPromptArguments() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExtractPromptDescription(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name                string
		content             string
		expectedDescription string
	}{
		{
			name:                "valid template with description",
			content:             "{{/* Template description */}}",
			expectedDescription: "Template description",
		},
		{
			name:                "valid template with description, comment starts with dash",
			content:             "{{- /* Template description */}}",
			expectedDescription: "Template description",
		},
		{
			name:                "valid template with description, comment ends with dash",
			content:             "{{/* Template description */ -}}",
			expectedDescription: "Template description",
		},
		{
			name:                "valid template with description, comment starts and ends with dash",
			content:             "{{- /* Template description */ -}}",
			expectedDescription: "Template description",
		},
		{
			name:                "template without description",
			content:             "Hello {{.name}}",
			expectedDescription: "",
		},
		{
			name:                "template with valid comment and trim",
			content:             "{{/* Comment */}}",
			expectedDescription: "Comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testFile := filepath.Join(tempDir, tt.name+".tmpl")
			if err := os.WriteFile(testFile, []byte(tt.content), 0644); err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}
			description, err := extractPromptDescription(testFile)
			if err != nil {
				t.Fatalf("parseTemplateFile() error = %v", err)
			}
			if description != tt.expectedDescription {
				t.Errorf("parseTemplateFile() description = %v, want %v", description, tt.expectedDescription)
			}
		})
	}
}

func TestFindUsedPartials(t *testing.T) {
	partials := map[string]string{
		"_header": "Header content with {{.role}}",
		"_footer": "Footer content with {{.conclusion}}",
		"_unused": "Unused content with {{.unused_var}}",
	}

	tests := []struct {
		name     string
		content  string
		expected map[string]string
	}{
		{
			name:     "no partials used",
			content:  "Just some {{.simple}} content",
			expected: map[string]string{},
		},
		{
			name:     "single partial used",
			content:  "{{template \"_header\" dict \"role\" .role}}",
			expected: map[string]string{"_header": "Header content with {{.role}}"},
		},
		{
			name:    "multiple partials used",
			content: "{{template \"_header\" .}} and {{template \"_footer\" .}}",
			expected: map[string]string{
				"_header": "Header content with {{.role}}",
				"_footer": "Footer content with {{.conclusion}}",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findUsedPartials(tt.content, partials)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("findUsedPartials() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoadPartials(t *testing.T) {
	tempDir := t.TempDir()

	// Create test partials
	partialFiles := map[string]string{
		"_header.tmpl": "{{/* Header partial */}}\nYou are {{.role}}",
		"_footer.tmpl": "{{/* Footer partial */}}\nEnd of prompt",
		"regular.tmpl": "{{/* Not a partial */}}\nRegular template",
	}

	for filename, content := range partialFiles {
		err := os.WriteFile(filepath.Join(tempDir, filename), []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to write test file %s: %v", filename, err)
		}
	}

	partials, err := loadPartials(tempDir)
	if err != nil {
		t.Fatalf("loadPartials() error = %v", err)
	}

	expected := map[string]string{
		"_header": "{{/* Header partial */}}\nYou are {{.role}}",
		"_footer": "{{/* Footer partial */}}\nEnd of prompt",
	}

	if !reflect.DeepEqual(partials, expected) {
		t.Errorf("loadPartials() = %v, want %v", partials, expected)
	}
}

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name           string
		templateName   string
		envVars        map[string]string
		expectedOutput string
		shouldError    bool
	}{
		{
			name:           "greeting template, env var not set",
			templateName:   "greeting",
			expectedOutput: "Hello {{ name }}!\nHave a great day!",
			shouldError:    false,
		},
		{
			name:         "greeting template",
			templateName: "greeting",
			envVars: map[string]string{
				"NAME": "John",
			},
			expectedOutput: "Hello John!\nHave a great day!",
			shouldError:    false,
		},
		{
			name:         "template with partials, some env vars not set",
			templateName: "multiple_partials",
			envVars: map[string]string{
				"TITLE":   "Test Document",
				"NAME":    "Bob",
				"VERSION": "1.0.0",
			},
			expectedOutput: "# Test Document\nCreated by: {{ author }}\n## Description\n{{ description }}\n## Details\nThis is a test template with multiple partials.\nHello Bob!\nVersion: 1.0.0",
			shouldError:    false,
		},
		{
			name:         "template with partials",
			templateName: "multiple_partials",
			envVars: map[string]string{
				"TITLE":       "Test Document",
				"AUTHOR":      "Test Author",
				"NAME":        "Bob",
				"DESCRIPTION": "This is a test description",
				"VERSION":     "1.0.0",
			},
			expectedOutput: "# Test Document\nCreated by: Test Author\n## Description\nThis is a test description\n## Details\nThis is a test template with multiple partials.\nHello Bob!\nVersion: 1.0.0",
			shouldError:    false,
		},
		{
			name:         "conditional greeting, show extra message true",
			templateName: "conditional_greeting",
			envVars: map[string]string{
				"NAME":                 "Alice",
				"SHOW_EXTRA_MESSAGE": "true",
			},
			expectedOutput: "Hello Alice!\nThis is an extra message just for you.\nHave a good day.",
			shouldError:    false,
		},
		{
			name:         "conditional greeting, show extra message false",
			templateName: "conditional_greeting",
			envVars: map[string]string{
				"NAME":                 "Bob",
				"SHOW_EXTRA_MESSAGE": "", 
			},
			expectedOutput: "Hello Bob!\nHave a good day.",
			shouldError:    false,
		},
		{
			name:           "non-existent template",
			templateName:   "non_existent_template",
			envVars:        map[string]string{},
			expectedOutput: "",
			shouldError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalEnv := make(map[string]string)
			for k := range tt.envVars {
				originalEnv[k] = os.Getenv(k)
			}
			defer func() {
				for k, v := range originalEnv {
					os.Setenv(k, v)
				}
			}()

			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			var buf bytes.Buffer
			err := renderTemplate(&buf, "./testdata", tt.templateName)

			if tt.shouldError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			output := normalizeNewlines(buf.String())
			if output != tt.expectedOutput {
				t.Errorf("expected output %q, got %q", tt.expectedOutput, output)
			}
		})
	}
}

func TestServerWithPrompt(t *testing.T) {
	ctx := context.Background()

	srv := mcptest.NewUnstartedServer(t)
	defer srv.Close()

	if err := buildPrompts(srv, "./testdata", slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("buildPrompts failed: %v", err)
	}

	err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name                string
		promptName          string
		promptArgs          map[string]string
		expectedDescription string
		expectedMessages    []mcp.PromptMessage
	}{
		{
			name:                "greeting prompt",
			promptName:          "greeting",
			promptArgs:          map[string]string{"name": "John"},
			expectedDescription: "Greeting standalone template with no partials",
			expectedMessages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Hello John!\nHave a great day!"),
				},
			},
		},
		{
			name:                "greeting with partials",
			promptName:          "greeting_with_partials",
			promptArgs:          map[string]string{"name": "Alice"},
			expectedDescription: "Greeting template with partial",
			expectedMessages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Hello Alice!\nWelcome to the system.\nHave a great day!"),
				},
			},
		},
		{
			name:       "template with multiple partials",
			promptName: "multiple_partials",
			promptArgs: map[string]string{
				"title":       "Test Document",
				"author":      "Test Author",
				"name":        "Bob",
				"description": "This is a test description",
				"version":     "1.0.0",
			},
			expectedDescription: "Template with multiple partials",
			expectedMessages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("# Test Document\nCreated by: Test Author\n## Description\nThis is a test description\n## Details\nThis is a test template with multiple partials.\nHello Bob!\nVersion: 1.0.0"),
				},
			},
		},
		{
			name:       "conditional greeting, show extra true",
			promptName: "conditional_greeting",
			promptArgs: map[string]string{
				"name":                 "Carlos",
				"show_extra_message": "true",
			},
			expectedDescription: "Conditional greeting template",
			expectedMessages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Hello Carlos!\nThis is an extra message just for you.\nHave a good day."),
				},
			},
		},
		{
			name:       "conditional greeting, show extra false",
			promptName: "conditional_greeting",
			promptArgs: map[string]string{
				"name":                 "Diana",
				"show_extra_message": "", 
			},
			expectedDescription: "Conditional greeting template",
			expectedMessages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.NewTextContent("Hello Diana!\nHave a good day."),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var getReq mcp.GetPromptRequest
			getReq.Params.Name = tt.promptName
			getReq.Params.Arguments = tt.promptArgs
			getResult, err := srv.Client().GetPrompt(ctx, getReq)
			if err != nil {
				t.Fatalf("GetPrompt failed: %v", err)
			}

			if getResult.Description != tt.expectedDescription {
				t.Errorf("Expected prompt description %q, got %q", tt.expectedDescription, getResult.Description)
			}

			if len(getResult.Messages) != len(tt.expectedMessages) {
				t.Fatalf("Expected %d messages, got %d", len(tt.expectedMessages), len(getResult.Messages))
			}

			for i, msg := range getResult.Messages {
				if msg.Role != tt.expectedMessages[i].Role {
					t.Errorf("Expected message role %q, got %q", tt.expectedMessages[i].Role, msg.Role)
				}
				content, ok := msg.Content.(mcp.TextContent)
				if !ok {
					t.Fatalf("Expected TextContent, got %T", msg.Content)
				}
				s := normalizeNewlines(content.Text)
				if s != tt.expectedMessages[i].Content.(mcp.TextContent).Text {
					t.Errorf("Expected message content %q, got %q", tt.expectedMessages[i].Content.(mcp.TextContent).Text, s)
				}
			}
		})
	}
}

var nlRegExp = regexp.MustCompile(`\n+`)

func normalizeNewlines(s string) string {
	s = nlRegExp.ReplaceAllString(s, "\n")
	return strings.TrimSpace(s)
}
