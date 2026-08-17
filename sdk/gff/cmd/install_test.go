package cmd

import (
	"testing"
)

func TestDeriveNamespace(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		// HTTPS URLs
		{
			name: "https_github",
			url:  "https://github.com/sfc-gh-eraigosa/dotfiles.git",
			want: "com.github.sfc-gh-eraigosa.dotfiles",
		},
		{
			name: "https_github_no_git",
			url:  "https://github.com/sfc-gh-eraigosa/dotfiles",
			want: "com.github.sfc-gh-eraigosa.dotfiles",
		},
		{
			name: "http_github",
			url:  "http://github.com/user/repo.git",
			want: "com.github.user.repo",
		},
		// SSH URLs
		{
			name: "ssh_shorthand",
			url:  "git@github.com:sfc-gh-eraigosa/dotfiles.git",
			want: "com.github.sfc-gh-eraigosa.dotfiles",
		},
		{
			name: "ssh_shorthand_no_git",
			url:  "git@github.com:user/repo",
			want: "com.github.user.repo",
		},
		{
			name: "ssh_full",
			url:  "ssh://git@github.com/user/repo.git",
			want: "com.github.user.repo",
		},
		// git:// protocol
		{
			name: "git_protocol",
			url:  "git://github.com/user/repo.git",
			want: "com.github.user.repo",
		},
		// Complex paths
		{
			name: "nested_path",
			url:  "https://github.com/org/group/project.git",
			want: "com.github.org.group.project",
		},
		{
			name: "custom_host",
			url:  "https://git.example.com/team/project.git",
			want: "com.example.git.team.project",
		},
		// Edge cases
		{
			name: "uppercase",
			url:  "https://GitHub.COM/User/Repo.git",
			want: "com.github.user.repo",
		},
		{
			name: "no_path",
			url:  "git@github.com",
			want: "com.github",
		},
		{
			name: "empty_url",
			url:  "",
			want: "",
		},
		{
			name: "invalid_host",
			url:  "https://@/path.git",
			want: "",
		},
		{
			name: "host_only",
			url:  "github.com",
			want: "com.github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveNamespace(tt.url)
			if got != tt.want {
				t.Errorf("deriveNamespace(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
