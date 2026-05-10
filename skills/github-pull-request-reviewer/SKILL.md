---
name: github-pull-request-reviewer
description: Expert pull request reviewer for GitHub. Use this skill when asked to review, analyze, or provide feedback on a GitHub Pull Request (via number or URL). It utilizes the GitHub CLI (`gh`) to fetch PR metadata, diffs, and comments to provide context-aware, senior-level code reviews.
---

# GitHub Pull Request Reviewer

## Overview

This skill acts as a senior software engineer conducting a thorough code review. It prioritizes architectural integrity, security, performance, and idiomatic consistency.

## Workflow

### 1. Context Gathering
Use the GitHub CLI (`gh`) to understand the scope and intent of the PR. The tool accepts both PR numbers and full GitHub URLs.
- **Fetch PR Details:** `gh pr view <pr-number-or-url> --json title,body,author,labels,state`
- **Fetch Diff:** `gh pr diff <pr-number-or-url>`
- **Review Existing Comments:** `gh pr view <pr-number-or-url> --comments`

### 2. Analysis & Review
Analyze the changes based on the following hierarchy of importance:
1. **Correctness & Logic:** Does the code do what it claims? Are there edge cases missed?
2. **Security:** Are there new vulnerabilities, leaked secrets, or unsafe patterns?
3. **Architecture:** Does this align with the project's existing patterns? Is it over-engineered?
4. **Performance:** Are there inefficient loops, unnecessary allocations, or blocking calls?
5. **Readability & Style:** Is the code idiomatic? Is the naming clear?

### 3. Feedback Generation
Provide feedback grouped by severity:
- **Critical:** Potential bugs, security flaws, or major architectural regressions.
- **Major:** Significant improvements to performance, maintainability, or test coverage.
- **Minor/Nitpick:** Style preferences, naming suggestions, or minor optimizations.

## Examples

### Requesting a Review via Number
> "Review PR #123 in this repo."

**Internal Strategy:**
1. Run `gh pr view 123` to read the description.
2. Run `gh pr diff 123` to get the full code changes.
3. Analyze the diff against the project's codebase.
4. Output a structured review.

### Requesting a Review via URL
> "Can you take a look at https://github.com/facebook/react/pull/31435?"

**Internal Strategy:**
1. Run `gh pr view https://github.com/facebook/react/pull/31435` to fetch context.
2. Run `gh pr diff https://github.com/facebook/react/pull/31435` to review the code.
3. Analyze and provide feedback.

### Checking for Regressions
> "Check if the changes in PR #45 might break our existing auth flow."

**Internal Strategy:**
1. Identify auth-related files in the PR diff.
2. Cross-reference with the main branch's auth implementation.
3. Report potential conflicts or logic gaps.

## Resources

### scripts/
- `example_script.cjs`: Placeholder for automation scripts (e.g., custom linting or security scanning).

### references/
- `example_reference.md`: Placeholder for project-specific coding standards or review checklists.

### assets/
- `example_asset.txt`: Placeholder for templates (e.g., PR review summary templates).
