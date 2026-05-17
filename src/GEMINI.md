# Source Code & Agent Skills (src)

This directory contains the source code for custom tools and specialized agent skills.

## Projects

- `tmux-mgr/`: A Go-based tool for managing tmux sessions, windows, and layouts, and orchestrating multi-agent teams.
  - [Project Documentation](./tmux-mgr/GEMINI.md)
  - [Agent Skill Instructions](./tmux-mgr/skill/SKILL.md)
- `ssh-host-finder/`: A skill for finding SSH hosts on the local network.
  - [Agent Skill Instructions](./ssh-host-finder/SKILL.md)

## Development Workflow

- Custom tools are typically developed in Go or Python.
- Agent skills are defined using `SKILL.md` files which provide specialized guidance.

### Testing & TDD Standards

All code modifications and new feature development under `src/` **MUST** follow a Test-Driven Development (TDD) workflow and adhere to standard testing practices for the language (e.g., standard Go testing patterns).

1. **Test First**: Start with the minimal tests needed for the feature. Include positive test cases, negative test cases, and edge cases (e.g., empty inputs). Add mocking if external dependencies are involved.
2. **Implement**: Add the new features to satisfy the tests.
3. **Validate**: Run the tests to validate the features.
4. **Iterate**: Debug what went wrong and iterate by adding more test cases until the desired result is achieved.

**Coverage Goal**: Aim for a minimum test coverage standard of **>60%** for all packages. When summarizing work, always include basic stats: added/removed lines, added/removed test cases, overall test coverage, and confidence level that the changes work.
