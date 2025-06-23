package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/fatih/color"
	"github.com/urfave/cli/v3"
)

var version = "dev"

const templateExt = ".tmpl"

// Color utility functions for consistent styling
var (
	// Status indicators
	successIcon = color.New(color.FgGreen, color.Bold).SprintFunc()
	errorIcon   = color.New(color.FgRed, color.Bold).SprintFunc()
	warningIcon = color.New(color.FgYellow, color.Bold).SprintFunc()
	infoIcon    = color.New(color.FgBlue, color.Bold).SprintFunc()
	
	// Text colors
	successText = color.New(color.FgGreen).SprintFunc()
	errorText   = color.New(color.FgRed).SprintFunc()
	warningText = color.New(color.FgYellow).SprintFunc()
	infoText    = color.New(color.FgBlue).SprintFunc()
	highlightText = color.New(color.FgCyan, color.Bold).SprintFunc()
	
	// Specific formatters
	templateText = color.New(color.FgMagenta, color.Bold).SprintFunc()
	envVarText   = color.New(color.FgCyan).SprintFunc()
	pathText     = color.New(color.FgBlue).SprintFunc()
)

func main() {
	cmd := &cli.Command{
		Name:    "mcp-prompt-engine",
		Usage:   "A Model Control Protocol server for dynamic prompt templates",
		Version: fmt.Sprintf("%s (Go %s)", version, runtime.Version()),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "prompts",
				Aliases: []string{"p"},
				Value:   "./prompts",
				Usage:   "Directory containing prompt template files",
				Sources: cli.EnvVars("MCP_PROMPTS_DIR"),
			},
			&cli.BoolFlag{
				Name:  "verbose",
				Usage: "Enable verbose output",
			},
			&cli.BoolFlag{
				Name:    "quiet",
				Aliases: []string{"q"},
				Usage:   "Suppress non-essential output",
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "serve",
				Aliases: []string{"s"},
				Usage:   "Start the MCP server",
				Action:  serveCommand,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "log-file",
						Usage: "Path to log file (if not specified, logs to stdout)",
					},
					&cli.BoolFlag{
						Name:  "disable-json-args",
						Usage: "Disable JSON parsing for arguments (use string-only mode)",
					},
				},
			},
			{
				Name:      "render",
				Aliases:   []string{"r"},
				Usage:     "Render a template to stdout",
				ArgsUsage: "<template_name>",
				Action:    renderCommand,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "show-vars",
						Usage: "Show template variables before rendering",
					},
					&cli.BoolFlag{
						Name:  "example",
						Usage: "Render with example values to show template structure",
					},
				},
			},
			{
				Name:    "list",
				Aliases: []string{"l"},
				Usage:   "List available templates",
				Action:  listCommand,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "detailed",
						Usage: "Show detailed information about templates",
					},
				},
			},
			{
				Name:      "validate",
				Aliases:   []string{"check"},
				Usage:     "Validate template syntax",
				ArgsUsage: "[template_name]",
				Action:    validateCommand,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "all",
						Usage: "Validate all templates",
					},
				},
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			// Validate prompts directory exists
			promptsDir := cmd.String("prompts")
			if _, err := os.Stat(promptsDir); os.IsNotExist(err) {
				return ctx, fmt.Errorf("prompts directory '%s' does not exist", promptsDir)
			}
			return ctx, nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

// serveCommand starts the MCP server
func serveCommand(ctx context.Context, cmd *cli.Command) error {
	promptsDir := cmd.String("prompts")
	logFile := cmd.String("log-file")
	enableJSONArgs := !cmd.Bool("disable-json-args")
	verbose := cmd.Bool("verbose")
	quiet := cmd.Bool("quiet")

	if !quiet {
		fmt.Printf("%s Loading templates from %s\n", successIcon("✓"), pathText(promptsDir))
	}

	if err := runMCPServer(promptsDir, logFile, enableJSONArgs, verbose, quiet); err != nil {
		return fmt.Errorf("%s: %w", errorText("failed to start MCP server"), err)
	}
	return nil
}

// renderCommand renders a template to stdout
func renderCommand(ctx context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() < 1 {
		return fmt.Errorf("template name is required\n\nUsage: %s render <template_name>", cmd.Root().Name)
	}

	promptsDir := cmd.String("prompts")
	templateName := cmd.Args().First()
	showVars := cmd.Bool("show-vars")
	example := cmd.Bool("example")
	verbose := cmd.Bool("verbose")

	if err := renderTemplate(os.Stdout, promptsDir, templateName, showVars, example, verbose); err != nil {
		return fmt.Errorf("%s '%s': %w", errorText("failed to render template"), templateText(templateName), err)
	}
	return nil
}

// listCommand lists available templates
func listCommand(ctx context.Context, cmd *cli.Command) error {
	promptsDir := cmd.String("prompts")
	detailed := cmd.Bool("detailed")
	verbose := cmd.Bool("verbose")

	if err := listTemplates(promptsDir, detailed, verbose); err != nil {
		return fmt.Errorf("failed to list templates: %w", err)
	}
	return nil
}

// validateCommand validates template syntax
func validateCommand(ctx context.Context, cmd *cli.Command) error {
	promptsDir := cmd.String("prompts")
	validateAll := cmd.Bool("all")
	verbose := cmd.Bool("verbose")

	var templateName string
	if cmd.Args().Len() > 0 {
		templateName = cmd.Args().First()
	}

	if !validateAll && templateName == "" {
		return fmt.Errorf("template name is required, or use --all to validate all templates\n\nUsage: %s validate <template_name> or %s validate --all", cmd.Root().Name, cmd.Root().Name)
	}

	if err := validateTemplates(promptsDir, templateName, validateAll, verbose); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

func runMCPServer(promptsDir string, logFile string, enableJSONArgs bool, verbose bool, quiet bool) error {
	// Configure logger
	logWriter := os.Stdout
	if logFile != "" {
		file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		defer func() { _ = file.Close() }()
		logWriter = file
	}
	logger := slog.New(slog.NewTextHandler(logWriter, nil))

	// Create PromptsServer instance
	promptsSrv, err := NewPromptsServer(promptsDir, enableJSONArgs, logger)
	if err != nil {
		return fmt.Errorf("new prompts server: %w", err)
	}

	if !quiet {
		// Count templates for feedback
		parser := &PromptsParser{}
		tmpl, err := parser.ParseDir(promptsDir)
		templateCount := 0
		if err == nil {
			for _, t := range tmpl.Templates() {
				if !strings.HasPrefix(t.Name(), "_") { // Skip partials
					templateCount++
				}
			}
		}
		
		fmt.Printf("%s Found %s templates\n", successIcon("✓"), highlightText(fmt.Sprintf("%d", templateCount)))
		fmt.Printf("%s Starting MCP server on %s\n", successIcon("✓"), infoText("stdio"))
		if verbose {
			fmt.Printf("%s JSON argument parsing: %s\n", infoIcon("ℹ"), highlightText(fmt.Sprintf("%t", enableJSONArgs)))
			if logFile != "" {
				fmt.Printf("%s Logging to: %s\n", infoIcon("ℹ"), pathText(logFile))
			}
		}
		fmt.Printf("%s Server ready - waiting for connections\n", successIcon("✓"))
	}
	defer func() {
		if closeErr := promptsSrv.Close(); closeErr != nil {
			logger.Error("Failed to close prompts server", "error", closeErr)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigChan
		logger.Info("Received shutdown signal, stopping server")
		cancel()
	}()

	return promptsSrv.ServeStdio(ctx, os.Stdin, os.Stdout)
}

// renderTemplate renders a specified template to stdout with resolved partials and environment variables
func renderTemplate(w io.Writer, promptsDir string, templateName string, showVars bool, example bool, verbose bool) error {
	parser := &PromptsParser{}

	tmpl, err := parser.ParseDir(promptsDir)
	if err != nil {
		return fmt.Errorf("parse all prompts: %w", err)
	}

	if tmpl.Lookup(templateName) == nil {
		if tmpl.Lookup(templateName+templateExt) == nil {
			// List available templates for better error message
			availableTemplates := []string{}
			for _, t := range tmpl.Templates() {
				name := t.Name()
				if !strings.HasPrefix(name, "_") { // Skip partials
					availableTemplates = append(availableTemplates, templateText(name))
				}
			}
			if len(availableTemplates) > 0 {
				return fmt.Errorf("template %s or %s not found\n\n%s:\n  %s", 
					errorText(templateName), errorText(templateName+templateExt),
					infoText("Available templates"), strings.Join(availableTemplates, "\n  "))
			}
			return fmt.Errorf("template %s or %s not found", errorText(templateName), errorText(templateName+templateExt))
		}
		templateName = templateName + templateExt
	}

	args, err := parser.ExtractPromptArgumentsFromTemplate(tmpl, templateName)
	if err != nil {
		return fmt.Errorf("extract template arguments: %w", err)
	}

	if showVars {
		fmt.Fprintf(os.Stderr, "Template variables for %s:\n", templateText(templateName))
		if len(args) > 0 {
			missingVars := []string{}
			foundVars := []string{}
			for _, arg := range args {
				envVarName := strings.ToUpper(arg)
				if _, exists := os.LookupEnv(envVarName); exists {
					foundVars = append(foundVars, fmt.Sprintf("  %s %s (from env: %s)", successIcon("✓"), highlightText(arg), envVarText(envVarName)))
				} else {
					missingVars = append(missingVars, fmt.Sprintf("  %s %s (env: %s)", warningIcon("•"), highlightText(arg), envVarText(envVarName)))
				}
			}
			if len(foundVars) > 0 {
				fmt.Fprintf(os.Stderr, "%s:\n%s\n", successText("Available"), strings.Join(foundVars, "\n"))
			}
			if len(missingVars) > 0 {
				fmt.Fprintf(os.Stderr, "%s:\n%s\n", warningText("Missing (will use placeholders)"), strings.Join(missingVars, "\n"))
			}
		} else {
			fmt.Fprintf(os.Stderr, "  %s\n", infoText("No variables required"))
		}
		fmt.Fprintf(os.Stderr, "%s: %s\n\n", infoText("Built-in"), highlightText("date"))
	}

	data := make(map[string]interface{})
	data["date"] = time.Now().Format("2006-01-02 15:04:05")

	// Add environment variables to data map
	for _, arg := range args {
		// Convert arg to TITLE_CASE for env var
		envVarName := strings.ToUpper(arg)
		if envValue, exists := os.LookupEnv(envVarName); exists {
			data[arg] = envValue
		} else if example {
			// Provide example values for better template structure visualization
			data[arg] = fmt.Sprintf("example_%s", arg)
		} else {
			data[arg] = "{{ " + arg + " }}"
		}
	}

	var result bytes.Buffer
	if err = tmpl.ExecuteTemplate(&result, templateName, data); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	_, err = w.Write(result.Bytes())
	return err
}

// listTemplates lists all available templates in the prompts directory
func listTemplates(promptsDir string, detailed bool, verbose bool) error {
	parser := &PromptsParser{}

	tmpl, err := parser.ParseDir(promptsDir)
	if err != nil {
		return fmt.Errorf("parse prompts directory: %w", err)
	}

	// Collect templates (exclude partials)
	templates := []string{}
	partials := []string{}
	
	for _, t := range tmpl.Templates() {
		name := t.Name()
		if strings.HasPrefix(name, "_") {
			partials = append(partials, name)
		} else {
			templates = append(templates, name)
		}
	}

	if len(templates) == 0 {
		fmt.Printf("%s No templates found in %s\n", warningIcon("⚠"), pathText(promptsDir))
		return nil
	}

	fmt.Printf("Available templates in %s:\n", pathText(promptsDir))
	
	for _, templateName := range templates {
		if detailed {
			// Extract variables for detailed view
			args, err := parser.ExtractPromptArgumentsFromTemplate(tmpl, templateName)
			if err != nil {
				fmt.Printf("  %s %s (%s)\n", errorIcon("✗"), templateText(templateName), errorText(fmt.Sprintf("error: %v", err)))
				continue
			}
			
			fmt.Printf("  %s %s", successIcon("✓"), templateText(templateName))
			if len(args) > 0 {
				fmt.Printf(" (%s: %s)", infoText(fmt.Sprintf("%d variables", len(args))), highlightText(strings.Join(args, ", ")))
			} else {
				fmt.Printf(" (%s)", infoText("no variables"))
			}
			fmt.Printf("\n")
		} else {
			// Extract description from first comment line
			description := extractTemplateDescription(tmpl, templateName)
			if description != "" {
				fmt.Printf("  %-20s - %s\n", templateText(templateName), description)
			} else {
				fmt.Printf("  %s\n", templateText(templateName))
			}
		}
	}

	if verbose && len(partials) > 0 {
		fmt.Printf("\n%s:\n", infoText("Available partials"))
		for _, partial := range partials {
			fmt.Printf("  %s\n", infoText(partial))
		}
	}

	return nil
}

// validateTemplates validates template syntax
func validateTemplates(promptsDir string, templateName string, validateAll bool, verbose bool) error {
	parser := &PromptsParser{}

	tmpl, err := parser.ParseDir(promptsDir)
	if err != nil {
		return fmt.Errorf("parse prompts directory: %w", err)
	}

	var templatesToValidate []string
	
	if validateAll {
		// Get all non-partial templates
		for _, t := range tmpl.Templates() {
			name := t.Name()
			if !strings.HasPrefix(name, "_") {
				templatesToValidate = append(templatesToValidate, name)
			}
		}
	} else {
		// Validate specific template
		if tmpl.Lookup(templateName) == nil {
			if tmpl.Lookup(templateName+templateExt) == nil {
				return fmt.Errorf("template %q or %q not found", templateName, templateName+templateExt)
			}
			templateName = templateName + templateExt
		}
		templatesToValidate = []string{templateName}
	}

	hasErrors := false
	
	for _, name := range templatesToValidate {
		// Try to extract arguments (this validates basic syntax)
		args, err := parser.ExtractPromptArgumentsFromTemplate(tmpl, name)
		if err != nil {
			fmt.Printf("%s %s - %s\n", errorIcon("✗"), templateText(name), errorText(fmt.Sprintf("Error: %v", err)))
			hasErrors = true
			continue
		}

		// Try to execute template with dummy data to catch runtime errors
		data := make(map[string]interface{})
		data["date"] = time.Now().Format("2006-01-02 15:04:05")
		for _, arg := range args {
			data[arg] = "test_value"
		}

		var result bytes.Buffer
		if err := tmpl.ExecuteTemplate(&result, name, data); err != nil {
			fmt.Printf("%s %s - %s\n", errorIcon("✗"), templateText(name), errorText(fmt.Sprintf("Execution error: %v", err)))
			hasErrors = true
			continue
		}

		if verbose {
			fmt.Printf("%s %s - %s", successIcon("✓"), templateText(name), successText("Valid"))
			if len(args) > 0 {
				fmt.Printf(" (%s: %s)", infoText("variables"), highlightText(strings.Join(args, ", ")))
			}
			fmt.Printf("\n")
		} else {
			fmt.Printf("%s %s - %s\n", successIcon("✓"), templateText(name), successText("Valid"))
		}
	}

	if hasErrors {
		return fmt.Errorf("some templates have validation errors")
	}

	return nil
}

// extractTemplateDescription extracts the description from the first comment in a template
func extractTemplateDescription(tmpl *template.Template, templateName string) string {
	// This is a simplified version - in a real implementation, you'd parse the template source
	// For now, return empty string as we don't have direct access to template source
	return ""
}
