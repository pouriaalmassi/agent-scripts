package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Execute sets up the root command tree and executes it.
func Execute() error {
	root := newRootCommand()
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	opts := &reviewViewOptions{}

	cmd := &cobra.Command{
		Use:           "gh-pr-review [<number> | <url>]",
		Short:         "PR review helper commands for gh",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && opts.Pull == 0 {
				return cmd.Help()
			}
			if len(args) > 0 {
				opts.Selector = args[0]
			}
			return runReviewView(cmd, opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Repository in 'owner/repo' format")
	cmd.Flags().IntVar(&opts.Pull, "pr", 0, "Pull request number")
	cmd.Flags().StringVar(&opts.Reviewer, "reviewer", "", "Filter to a specific reviewer (login)")
	cmd.Flags().StringSliceVar(&opts.States, "states", nil, "Comma-separated review states (APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED)")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Only include unresolved threads")
	cmd.Flags().BoolVar(&opts.NotOutdated, "not_outdated", false, "Exclude outdated threads")
	cmd.Flags().IntVar(&opts.TailReplies, "tail", 0, "Limit to the last N replies per thread (0 = all)")
	cmd.Flags().BoolVar(&opts.IncludeCommentNodeID, "include-comment-node-id", false, "Include comment_node_id fields for parent comments and replies")

	cmd.AddCommand(newCommentsCommand())
	cmd.AddCommand(newReviewCommand())
	cmd.AddCommand(newThreadsCommand())

	return cmd
}

// ExecuteOrExit runs the command tree and exits with a non-zero status on error.
func ExecuteOrExit() {
	if err := Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
