package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shhac/git-wt/internal/wt"
)

var (
	newParentDir      string
	newFromRef        string
	newNoFetch        bool
	newNoCopy         bool
	newCopyFileConfig string
)

var newCmd = &cobra.Command{
	Use:   "new <branch>",
	Short: "Create a new worktree with branch <branch>",
	Long: "Create a new worktree at <repo>/.worktrees/<branch>/ (or --parent-dir).\n" +
		"By default, fetches origin and branches from its default branch as it\n" +
		"is now. --from <ref> picks another start point (a <remote>/<x> ref\n" +
		"fetches that remote first). --no-fetch skips the fetch. The new branch\n" +
		"has no upstream. Then copies project-local files according to\n" +
		"<repo>/.git-wt-copy-files (override path with --copy-file-config).\n" +
		"Built-in defaults are used when no spec file is present. Skip the copy\n" +
		"entirely with --no-copy.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		branch := args[0]

		if err := wt.ValidateBranchName(branch); err != nil {
			return err
		}

		repo, err := requireMutableRepo(ctx)
		if err != nil {
			return err
		}
		exists, err := wt.BranchExists(ctx, "", branch)
		if err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("branch %q already exists", branch)
		}

		base, err := resolveNewBase(ctx, newFromRef, newNoFetch)
		if err != nil {
			return err
		}
		parent, err := resolveParentDir(ctx, repo.MainRoot, newParentDir)
		if err != nil {
			return err
		}
		path, err := prepareWorktreeSite(parent, branch, "branch name")
		if err != nil {
			return err
		}

		if err := createWorktree(ctx, path, branch, base.Ref); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, describeBase(base))
		return finalizeWorktreeSite(ctx, repo, path, parent, newNoCopy, newCopyFileConfig)
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
	newCmd.Flags().StringVarP(&newParentDir, "parent-dir", "p", "", "parent directory for the worktree (default: <repo>/.worktrees/)")
	newCmd.Flags().StringVar(&newFromRef, "from", "", "start point (default: origin's default branch, fetched first; HEAD without a remote)")
	newCmd.Flags().BoolVar(&newNoFetch, "no-fetch", false, "branch from the refs as they are, without fetching the remote")
	newCmd.Flags().BoolVar(&newNoCopy, "no-copy", false, "skip copying project config files")
	newCmd.Flags().StringVar(&newCopyFileConfig, "copy-file-config", "", "path to copy spec (default: <repo>/.git-wt-copy-files)")
}
