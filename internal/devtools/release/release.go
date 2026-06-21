// Package release implements interactive release automation for gomailtesttool.
// It replicates run-interactive-release.ps1 in portable Go, and fixes a latent
// bug in that script which read/wrote the legacy src\VERSION path instead of
// internal/common/version/version.go.
package release

import (
	"fmt"
	"io"
	"path/filepath"
)

// Options controls optional steps in the release flow.
type Options struct {
	ProjectRoot      string
	VersionFilePath  string // relative to ProjectRoot; default: internal/common/version/version.go
	SkipSecurityScan bool
	SkipPush         bool
	NoDevBranch      bool
	DryRun           bool
	In               io.Reader
	Out              io.Writer
}

func (o *Options) versionFile() string {
	if o.VersionFilePath != "" {
		return filepath.Join(o.ProjectRoot, o.VersionFilePath)
	}
	return filepath.Join(o.ProjectRoot, "internal", "common", "version", "version.go")
}

func (o *Options) printf(format string, args ...any) {
	if o.Out != nil {
		fmt.Fprintf(o.Out, format, args...)
	}
}

func (o *Options) dry(action string, fn func() error) error {
	if o.DryRun {
		o.printf("[dry-run] %s\n", action)
		return nil
	}
	return fn()
}

// Run executes the interactive release workflow.
func Run(opts Options) error {
	w := opts.Out
	r := opts.In

	opts.printf("\n=== gomailtesttool release tool ===\n\n")

	// Step 1: Git status check
	opts.printf("Step 1/13: Checking git status...\n")
	dirty, statusOut, err := Status()
	if err != nil {
		return fmt.Errorf("step 1 git status: %w", err)
	}
	if dirty {
		opts.printf("WARNING: Working tree has uncommitted changes:\n%s\n\n", statusOut)
		cont, err := PromptYesNo(w, r, "Continue anyway?", false)
		if err != nil || !cont {
			return fmt.Errorf("aborted: uncommitted changes")
		}
	} else {
		opts.printf("  Working tree is clean.\n")
	}

	// Step 2: Security scan
	if !opts.SkipSecurityScan {
		opts.printf("\nStep 2/13: Scanning for secrets...\n")
		findings, err := ScanFiles(opts.ProjectRoot)
		if err != nil {
			return fmt.Errorf("step 2 security scan: %w", err)
		}
		if len(findings) > 0 {
			opts.printf("WARNING: %d potential secret(s) found:\n", len(findings))
			for _, f := range findings {
				opts.printf("  %s:%d [%s] %s\n", f.File, f.Line, f.Kind, f.Content)
			}
			opts.printf("\n")
			cont, err := PromptYesNo(w, r, "Continue despite findings?", false)
			if err != nil || !cont {
				return fmt.Errorf("aborted: potential secrets detected")
			}
		} else {
			opts.printf("  No secrets detected.\n")
		}
	} else {
		opts.printf("\nStep 2/13: Security scan skipped (--skip-security-scan).\n")
	}

	// Step 3: Read current version
	opts.printf("\nStep 3/13: Reading current version...\n")
	currentVer, err := ReadVersion(opts.versionFile())
	if err != nil {
		return fmt.Errorf("step 3 read version: %w", err)
	}
	opts.printf("  Current version: %s\n", currentVer)

	// Step 4: Prompt for new version
	opts.printf("\nStep 4/13: Select new version...\n")
	newVer, err := PromptVersion(w, r, currentVer)
	if err != nil {
		return fmt.Errorf("step 4 version selection: %w", err)
	}
	opts.printf("  New version: %s\n", newVer)

	// Step 5: Write version
	opts.printf("\nStep 5/13: Updating version file...\n")
	if err := opts.dry(fmt.Sprintf("WriteVersion(%s)", newVer), func() error {
		return WriteVersion(opts.versionFile(), newVer)
	}); err != nil {
		return fmt.Errorf("step 5 write version: %w", err)
	}
	opts.printf("  Updated %s\n", opts.versionFile())

	// Step 6: Collect changelog sections
	opts.printf("\nStep 6/13: Changelog entry for v%s\n", newVer)
	sections := Sections{
		Added:    PromptLines(w, r, "Added"),
		Changed:  PromptLines(w, r, "Changed"),
		Fixed:    PromptLines(w, r, "Fixed"),
		Security: PromptLines(w, r, "Security"),
	}
	changelogPath := EntryPath(opts.ProjectRoot, newVer)
	if err := opts.dry(fmt.Sprintf("CreateEntry(%s)", changelogPath), func() error {
		return CreateEntry(changelogPath, newVer, sections)
	}); err != nil {
		return fmt.Errorf("step 6 create changelog: %w", err)
	}

	// Step 7: Open changelog in editor
	opts.printf("\nStep 7/13: Opening changelog in editor...\n")
	if !opts.DryRun {
		if err := OpenInEditor(changelogPath); err != nil {
			opts.printf("WARNING: editor failed: %v — continuing\n", err)
		}
	} else {
		opts.printf("[dry-run] OpenInEditor(%s)\n", changelogPath)
	}

	// Step 8: Git commit
	opts.printf("\nStep 8/13: Committing changes...\n")
	commitMsg := fmt.Sprintf("Bump version to %s\n\nCo-Authored-By: Claude Sonnet 4.6 (1M context) <noreply@anthropic.com>", newVer)
	if err := opts.dry("git add + commit", func() error {
		if err := Add(opts.versionFile(), changelogPath); err != nil {
			return err
		}
		return Commit(commitMsg)
	}); err != nil {
		return fmt.Errorf("step 8 commit: %w", err)
	}

	// Step 9: Push branch
	if !opts.SkipPush {
		opts.printf("\nStep 9/13: Pushing branch...\n")
		branch, err := CurrentBranch()
		if err != nil {
			return fmt.Errorf("step 9 get branch: %w", err)
		}
		doPush, err := PromptYesNo(w, r, fmt.Sprintf("Push branch %q to origin?", branch), true)
		if err != nil {
			return err
		}
		if doPush {
			if err := opts.dry(fmt.Sprintf("git push origin %s", branch), func() error {
				return Push("origin", branch)
			}); err != nil {
				return fmt.Errorf("step 9 push: %w", err)
			}
		}
	} else {
		opts.printf("\nStep 9/13: Push skipped (--skip-push).\n")
	}

	// Step 10: Create and push git tag
	opts.printf("\nStep 10/13: Creating git tag v%s (triggers GitHub Actions)...\n", newVer)
	doTag, err := PromptYesNo(w, r, fmt.Sprintf("Create and push tag v%s?", newVer), true)
	if err != nil {
		return err
	}
	if doTag {
		tagName := "v" + newVer
		if err := opts.dry(fmt.Sprintf("git tag %s", tagName), func() error {
			return Tag(tagName)
		}); err != nil {
			return fmt.Errorf("step 10 tag: %w", err)
		}
		if !opts.SkipPush {
			if err := opts.dry(fmt.Sprintf("git push origin %s", tagName), func() error {
				return PushTag("origin", tagName)
			}); err != nil {
				return fmt.Errorf("step 10 push tag: %w", err)
			}
			opts.printf("  Tag pushed — GitHub Actions should start shortly.\n")
		}
	}

	// Step 11: Optional PR creation
	opts.printf("\nStep 11/13: Pull request (optional)...\n")
	branch, _ := CurrentBranch()
	if branch != "main" && branch != "master" && ghAvailable() {
		doPR, err := PromptYesNo(w, r, fmt.Sprintf("Create PR to merge %q into main?", branch), false)
		if err != nil {
			return err
		}
		if doPR {
			prURL, err := opts.dry2("gh pr create", func() (string, error) {
				return CreatePR(
					fmt.Sprintf("Release v%s", newVer),
					fmt.Sprintf("Release v%s\n\nSee ChangeLog/%s.md for details.", newVer, newVer),
					"main",
				)
			})
			if err != nil {
				opts.printf("WARNING: PR creation failed: %v\n", err)
			} else {
				opts.printf("  PR created: %s\n", prURL)
			}
		}
	} else if !ghAvailable() {
		opts.printf("  'gh' CLI not found — skipping PR creation.\n")
	}

	// Step 12: Optional GitHub Actions monitoring
	opts.printf("\nStep 12/13: GitHub Actions status (optional)...\n")
	doRuns, err := PromptYesNo(w, r, "Show recent workflow runs?", false)
	if err != nil {
		return err
	}
	if doRuns {
		runs, err := ListRuns(5)
		if err != nil {
			opts.printf("WARNING: %v\n", err)
		} else {
			opts.printf("%s\n", runs)
		}
	}

	// Step 13: Prepare next dev cycle
	if !opts.NoDevBranch {
		opts.printf("\nStep 13/13: Prepare next development cycle...\n")
		nextVer, err := SuggestNextPatch(newVer)
		if err != nil {
			return fmt.Errorf("step 13 suggest next patch: %w", err)
		}
		devBranch := "b" + nextVer
		opts.printf("  Next version will be: %s\n", nextVer)
		doNext, err := PromptYesNo(w, r, fmt.Sprintf("Create dev branch %q and bump version to %s?", devBranch, nextVer), true)
		if err != nil {
			return err
		}
		if doNext {
			if err := opts.dry(fmt.Sprintf("git checkout -b %s", devBranch), func() error {
				return CheckoutNewBranch(devBranch)
			}); err != nil {
				return fmt.Errorf("step 13 checkout dev branch: %w", err)
			}
			if err := opts.dry(fmt.Sprintf("WriteVersion(%s)", nextVer), func() error {
				return WriteVersion(opts.versionFile(), nextVer)
			}); err != nil {
				return fmt.Errorf("step 13 write next version: %w", err)
			}
			nextCommitMsg := fmt.Sprintf("Bump version to %s\n\nCo-Authored-By: Claude Sonnet 4.6 (1M context) <noreply@anthropic.com>", nextVer)
			if err := opts.dry("git add + commit dev version", func() error {
				if err := Add(opts.versionFile()); err != nil {
					return err
				}
				return Commit(nextCommitMsg)
			}); err != nil {
				return fmt.Errorf("step 13 commit dev version: %w", err)
			}
			if !opts.SkipPush {
				doPushDev, err := PromptYesNo(w, r, fmt.Sprintf("Push dev branch %q to origin?", devBranch), true)
				if err != nil {
					return err
				}
				if doPushDev {
					if err := opts.dry(fmt.Sprintf("git push origin %s", devBranch), func() error {
						return Push("origin", devBranch)
					}); err != nil {
						return fmt.Errorf("step 13 push dev branch: %w", err)
					}
				}
			}
		}
	} else {
		opts.printf("\nStep 13/13: Dev branch skipped (--no-dev-branch).\n")
	}

	opts.printf("\n=== Release v%s complete ===\n", newVer)
	return nil
}

// dry2 is like dry but for functions that return (string, error).
func (o *Options) dry2(action string, fn func() (string, error)) (string, error) {
	if o.DryRun {
		o.printf("[dry-run] %s\n", action)
		return "", nil
	}
	return fn()
}
