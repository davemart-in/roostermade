package cli

import "github.com/spf13/cobra"

const manText = `Recall Command Reference

Core
  recall init                      Guided setup + context capture
  recall status                    Show note/summary/doc counts
  recall man                       Show this command reference
  recall config                    Interactive config/doc editor
  recall config get <key>          Print config value
  recall config set <key> <value>  Set writable config value
  recall context                   Print context snapshot (context + summaries + docs)
  recall doctor                    Run health checks (project/db/summarizer)
  recall mcp                       Run MCP server over stdio
  recall export                    Export recall data to zip
  recall import <zipfile>          Import recall data from zip

Note
  recall note add "<content>" [--llm <provider>] [--model <model>]
                                   Add a note
  recall note list                 List notes
  recall note get <id>             Get note details
  recall note search <query>       Search notes by content

Summary
  recall summary add               Summarize unsummarized notes
  recall summary list              List summaries
  recall summary get <id>          Get summary details
  recall summary search <query>    Search summaries by body

Doc
  recall doc add <name>            Create and register a doc
  recall doc edit <name>           Open a doc in $EDITOR
  recall doc list                  List registered docs
`

func newManCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "man",
		Short: "Show full command reference",
		Run: func(cmd *cobra.Command, _ []string) {
			cmd.Print(manText)
		},
	}
}
