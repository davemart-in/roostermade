package cli

import "github.com/spf13/cobra"

const manText = `Recall Command Reference

Core
  recall init                      Guided setup + context/doc planning
  recall status                    Show thought/summary/doc counts
  recall man                       Show this command reference
  recall config                    Interactive config and doc selection editor
  recall context                   Print .recall/context.md

Thought
  recall thought add "<content>" [--llm <provider>] [--model <model>]
                                   Add a thought
  recall thought list              List thoughts
  recall thought get <id>          Get thought details

Summary
  recall summary add               Summarize unsummarized thoughts
  recall summary list              List summaries
  recall summary get <id>          Get summary details

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
