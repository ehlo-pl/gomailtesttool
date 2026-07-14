package serve

// Config holds HTTP server configuration for the serve subcommand.
type Config struct {
	Port      int    // listen port (default 8080)
	Listen    string // bind address; defaults to 127.0.0.1, use 0.0.0.0 for all interfaces
	APIKey    string // required X-API-Key header value
	EnableMCP bool   // mount the MCP streamable-HTTP endpoint at /mcp (behind X-API-Key)
}
