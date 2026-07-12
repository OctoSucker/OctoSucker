// Package opencli adapts OpenCLI's machine-readable command catalog into
// MCP-shaped tools. The model never constructs process argv directly.
package opencli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	types "github.com/OctoSucker/octosucker/internal/toolcontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

const providerName = "opencli"

type Provider struct {
	executable string
	timeout    time.Duration
	tools      map[string]*mcp.Tool
	commands   map[string]commandSpec
}

type Options struct {
	Command  string
	Commands map[string][]string
	Timeout  time.Duration
}

type helpDocument struct {
	Site     string        `yaml:"site"`
	Commands []helpCommand `yaml:"commands"`
}

type helpCommand struct {
	Name           string         `yaml:"name"`
	Usage          string         `yaml:"usage"`
	Access         string         `yaml:"access"`
	Description    string         `yaml:"description"`
	Browser        bool           `yaml:"browser"`
	Positionals    []helpArgument `yaml:"positionals"`
	CommandOptions []helpArgument `yaml:"command_options"`
}

type helpArgument struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Default    any    `yaml:"default"`
	Help       string `yaml:"help"`
	Required   bool   `yaml:"required"`
	Positional bool   `yaml:"positional"`
	Choices    []any  `yaml:"choices"`
}

type commandSpec struct {
	ToolName    string
	Site        string
	Command     string
	Access      string
	Description string
	Browser     bool
	Positionals []argumentSpec
	Options     []argumentSpec
}

type argumentSpec struct {
	Property string
	CLIName  string
	Type     string
	Required bool
	Default  any
}

func NewProvider(ctx context.Context, options Options) (*Provider, error) {
	executable, err := exec.LookPath(strings.TrimSpace(options.Command))
	if err != nil {
		return nil, fmt.Errorf("opencli provider: executable %q is unavailable: %w", options.Command, err)
	}
	p := &Provider{
		executable: executable,
		timeout:    options.Timeout,
		tools:      make(map[string]*mcp.Tool),
		commands:   make(map[string]commandSpec),
	}
	if p.timeout <= 0 {
		p.timeout = 90 * time.Second
	}

	sites := make([]string, 0, len(options.Commands))
	for site := range options.Commands {
		sites = append(sites, site)
	}
	sort.Strings(sites)
	for _, site := range sites {
		if err := p.loadSite(ctx, site, options.Commands[site]); err != nil {
			return nil, err
		}
	}
	if len(p.tools) == 0 {
		return nil, fmt.Errorf("opencli provider: command allowlist exposed no tools")
	}
	return p, nil
}

func (p *Provider) Name() (string, string) {
	return providerName, "Schema-generated OpenCLI website tools with deterministic argv compilation."
}

func (p *Provider) HasTool(name string) bool {
	_, ok := p.tools[strings.TrimSpace(name)]
	return ok
}

func (p *Provider) Tool(name string) (*mcp.Tool, error) {
	tool, ok := p.tools[strings.TrimSpace(name)]
	if !ok {
		return nil, fmt.Errorf("opencli provider: unknown tool %q", name)
	}
	return tool, nil
}

func (p *Provider) ToolList(context.Context) ([]*mcp.Tool, error) {
	names := make([]string, 0, len(p.tools))
	for name := range p.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]*mcp.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, p.tools[name])
	}
	return out, nil
}

func (p *Provider) Invoke(ctx context.Context, toolName string, arguments map[string]any) (types.ToolResult, error) {
	spec, ok := p.commands[strings.TrimSpace(toolName)]
	if !ok {
		err := fmt.Errorf("opencli provider: unknown tool %q", toolName)
		return types.ToolResult{Err: err}, err
	}
	argv, err := compileArguments(spec, arguments)
	if err != nil {
		err = fmt.Errorf("opencli provider: %s: %w", toolName, err)
		return types.ToolResult{Err: err}, err
	}
	argv = append([]string{spec.Site, spec.Command}, argv...)
	argv = append(argv, "-f", "json")

	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, p.executable, argv...)
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			err = runCtx.Err()
		}
		callErr := commandError(spec, stdout.String(), stderr.String(), err)
		return types.ToolResult{Err: callErr}, callErr
	}

	raw := bytes.TrimSpace(stdout.Bytes())
	if len(raw) == 0 {
		return types.ToolResult{Output: nil}, nil
	}
	var output any
	if err := json.Unmarshal(raw, &output); err != nil {
		parseErr := fmt.Errorf("opencli provider: %s returned invalid JSON: %w", toolName, err)
		return types.ToolResult{Err: parseErr}, parseErr
	}
	return types.ToolResult{Output: output}, nil
}

func (p *Provider) Assess(toolName string, _ map[string]any) types.ToolPolicy {
	spec, ok := p.commands[strings.TrimSpace(toolName)]
	if !ok {
		return types.ToolPolicy{
			Capabilities: []string{"tool"},
			Risk:         "low",
			Summary:      "unknown OpenCLI tool",
			OutputTrust:  types.TrustUntrustedData,
		}
	}
	capabilities := []string{"network"}
	if spec.Browser {
		capabilities = append(capabilities, "browser_session")
	}
	risk := "medium"
	summary := "reads website data through OpenCLI"
	if strings.EqualFold(spec.Access, "write") {
		capabilities = append(capabilities, "external_write")
		risk = "high"
		summary = "mutates an external website through OpenCLI"
	}
	return types.ToolPolicy{
		Capabilities: capabilities,
		Risk:         risk,
		Summary:      summary,
		OutputTrust:  types.TrustUntrustedData,
	}
}

func (p *Provider) loadSite(ctx context.Context, site string, allowlist []string) error {
	doc, err := p.readSiteHelp(ctx, site)
	if err != nil {
		return err
	}
	available := make(map[string]helpCommand, len(doc.Commands))
	for _, command := range doc.Commands {
		name := strings.TrimSpace(command.Name)
		if name != "" {
			available[name] = command
		}
	}
	for _, commandName := range allowlist {
		command, ok := available[commandName]
		if !ok {
			return fmt.Errorf("opencli provider: site %q does not expose allowed command %q", site, commandName)
		}
		spec, tool, err := buildTool(site, command)
		if err != nil {
			return err
		}
		if _, exists := p.tools[tool.Name]; exists {
			return fmt.Errorf("opencli provider: duplicate generated tool %q", tool.Name)
		}
		p.tools[tool.Name] = tool
		p.commands[tool.Name] = spec
	}
	return nil
}

func (p *Provider) readSiteHelp(ctx context.Context, site string) (helpDocument, error) {
	runCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, p.executable, site, "--help", "-f", "yaml")
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if runCtx.Err() != nil {
			err = runCtx.Err()
		}
		return helpDocument{}, fmt.Errorf("opencli provider: inspect site %q: %s", site, compactError(stdout.String(), stderr.String(), err))
	}
	var doc helpDocument
	if err := yaml.Unmarshal(stdout.Bytes(), &doc); err != nil {
		return helpDocument{}, fmt.Errorf("opencli provider: parse help for site %q: %w", site, err)
	}
	if len(doc.Commands) == 0 {
		return helpDocument{}, fmt.Errorf("opencli provider: site %q returned no command metadata", site)
	}
	return doc, nil
}

func buildTool(site string, command helpCommand) (commandSpec, *mcp.Tool, error) {
	spec := commandSpec{
		ToolName:    "opencli_" + identifier(site) + "_" + identifier(command.Name),
		Site:        site,
		Command:     strings.TrimSpace(command.Name),
		Access:      strings.TrimSpace(command.Access),
		Description: strings.TrimSpace(command.Description),
		Browser:     command.Browser,
	}
	properties := make(map[string]any)
	required := make([]string, 0)
	seen := make(map[string]struct{})
	for _, input := range command.Positionals {
		arg, schema, err := buildArgument(input, true)
		if err != nil {
			return commandSpec{}, nil, fmt.Errorf("opencli provider: %s %s positional: %w", site, command.Name, err)
		}
		if _, exists := seen[arg.Property]; exists {
			return commandSpec{}, nil, fmt.Errorf("opencli provider: %s %s duplicate input %q", site, command.Name, arg.Property)
		}
		seen[arg.Property] = struct{}{}
		properties[arg.Property] = schema
		spec.Positionals = append(spec.Positionals, arg)
		if arg.Required {
			required = append(required, arg.Property)
		}
	}
	for _, input := range command.CommandOptions {
		arg, schema, err := buildArgument(input, false)
		if err != nil {
			return commandSpec{}, nil, fmt.Errorf("opencli provider: %s %s option: %w", site, command.Name, err)
		}
		if _, exists := seen[arg.Property]; exists {
			return commandSpec{}, nil, fmt.Errorf("opencli provider: %s %s duplicate input %q", site, command.Name, arg.Property)
		}
		seen[arg.Property] = struct{}{}
		properties[arg.Property] = schema
		spec.Options = append(spec.Options, arg)
		if arg.Required {
			required = append(required, arg.Property)
		}
	}
	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	description := fmt.Sprintf("OpenCLI %s %s", site, command.Name)
	if spec.Description != "" {
		description += ": " + spec.Description
	}
	if spec.Access != "" {
		description += " Access: " + spec.Access + "."
	}
	return spec, &mcp.Tool{Name: spec.ToolName, Description: description, InputSchema: schema}, nil
}

func buildArgument(input helpArgument, positional bool) (argumentSpec, map[string]any, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return argumentSpec{}, nil, fmt.Errorf("input name is required")
	}
	typeName := schemaType(input.Type, input.Default, input.Choices)
	property := identifier(name)
	schema := map[string]any{"type": typeName}
	if help := strings.TrimSpace(input.Help); help != "" {
		schema["description"] = help
	}
	if len(input.Choices) > 0 {
		schema["enum"] = input.Choices
	}
	if input.Default != nil {
		schema["default"] = input.Default
	}
	return argumentSpec{
		Property: property,
		CLIName:  name,
		Type:     typeName,
		Required: input.Required,
		Default:  input.Default,
	}, schema, nil
}

func compileArguments(spec commandSpec, arguments map[string]any) ([]string, error) {
	if arguments == nil {
		arguments = map[string]any{}
	}
	argv := make([]string, 0, len(arguments)*2)
	missingOptional := false
	for _, input := range spec.Positionals {
		value, exists := arguments[input.Property]
		if !exists || value == nil {
			if input.Required {
				return nil, fmt.Errorf("missing required positional %q", input.Property)
			}
			missingOptional = true
			continue
		}
		if missingOptional {
			return nil, fmt.Errorf("positional %q cannot follow an omitted optional positional", input.Property)
		}
		text, err := scalarText(value, input.Type)
		if err != nil {
			return nil, fmt.Errorf("positional %q: %w", input.Property, err)
		}
		argv = append(argv, text)
	}
	for _, input := range spec.Options {
		value, exists := arguments[input.Property]
		if !exists || value == nil {
			continue
		}
		flag := "--" + input.CLIName
		if input.Type == "boolean" {
			enabled, ok := value.(bool)
			if !ok {
				return nil, fmt.Errorf("option %q must be boolean", input.Property)
			}
			if enabled {
				argv = append(argv, flag)
			} else if defaultEnabled, ok := input.Default.(bool); ok && defaultEnabled {
				argv = append(argv, flag+"=false")
			}
			continue
		}
		text, err := scalarText(value, input.Type)
		if err != nil {
			return nil, fmt.Errorf("option %q: %w", input.Property, err)
		}
		argv = append(argv, flag, text)
	}
	return argv, nil
}

func scalarText(value any, typeName string) (string, error) {
	switch typeName {
	case "integer":
		switch n := value.(type) {
		case int:
			return strconv.Itoa(n), nil
		case int64:
			return strconv.FormatInt(n, 10), nil
		case float64:
			if n != float64(int64(n)) {
				return "", fmt.Errorf("must be an integer")
			}
			return strconv.FormatInt(int64(n), 10), nil
		default:
			return "", fmt.Errorf("must be an integer")
		}
	case "number":
		switch n := value.(type) {
		case float64:
			return strconv.FormatFloat(n, 'f', -1, 64), nil
		case int:
			return strconv.Itoa(n), nil
		default:
			return "", fmt.Errorf("must be a number")
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return "", fmt.Errorf("must be a string")
		}
		return text, nil
	default:
		return "", fmt.Errorf("unsupported scalar type %q", typeName)
	}
}

func schemaType(raw string, defaultValue any, choices []any) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "int", "integer":
		return "integer"
	case "float", "double", "number":
		return "number"
	case "bool", "boolean":
		return "boolean"
	case "str", "string", "":
		if defaultValue != nil {
			return inferredType(defaultValue)
		}
		if len(choices) > 0 {
			return inferredType(choices[0])
		}
		return "string"
	default:
		return "string"
	}
}

func inferredType(value any) string {
	switch value.(type) {
	case bool:
		return "boolean"
	case int, int64:
		return "integer"
	case float32, float64:
		return "number"
	default:
		return "string"
	}
}

func identifier(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func commandError(spec commandSpec, stdout, stderr string, runErr error) error {
	return fmt.Errorf("opencli provider: %s %s failed: %s", spec.Site, spec.Command, compactError(stdout, stderr, runErr))
}

func compactError(stdout, stderr string, runErr error) string {
	parts := make([]string, 0, 3)
	if text := strings.TrimSpace(stderr); text != "" {
		parts = append(parts, text)
	}
	if text := strings.TrimSpace(stdout); text != "" {
		parts = append(parts, text)
	}
	if runErr != nil {
		parts = append(parts, runErr.Error())
	}
	text := strings.Join(parts, " | ")
	runes := []rune(text)
	if len(runes) > 1200 {
		text = string(runes[:1200]) + "..."
	}
	return text
}
