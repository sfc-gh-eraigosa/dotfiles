# gss (Git Safe Sync)

gss is a specialized tool for safely synchronizing and pushing changes to Git repositories. It is designed to prevent data loss by automating safety backups before performing destructive actions like rebases or forced pushes.

## Core Purpose
The primary goal of gss is to provide a "safety net" for developers and autonomous agents working in Git. It ensures that every synchronization attempt is preceded by a local backup branch, making it easy to recover if a rebase goes wrong.

## Commands

- gss status: Summarizes changes in a repository.
- gss push: Safely pushes changes.
- gss sync: Synchronizes with the origin using rebase.
- gss backup: Explicitly creates a safety backup branch.
- gss scan [dir]: Scans a directory for any Git repositories that have uncommitted changes.
- gss pr: Creates a feature branch, pushes it, and opens a Pull Request via the gh CLI.

## Usage

Run from within a repository:
gss push

Run from outside a repository:
gss status --repo ~/git/project-a

Scan for dirty repositories:
gss scan ~/git

## Safety Features
- Auto-Backup: Every push creates a backup/gss-TIMESTAMP branch.
- Rebase by Default: Encourages a clean history by using pull --rebase.
- Introspection: Built-in scanning for dirty files across multiple projects.
