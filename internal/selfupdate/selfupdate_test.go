package selfupdate

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name        string
		exe         string
		mainVersion string
		buildBy     string
		vcsStamped  bool
		want        Channel
	}{
		{
			name:        "scoop apps path wins over goreleaser stamp",
			exe:         `C:\Users\dev\scoop\apps\jira\0.6.5\jira.exe`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelScoop,
		},
		{
			name:        "scoop shims path",
			exe:         `D:\tools\scoop\shims\jira.exe`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelScoop,
		},
		{
			name:        "scoop path is case-insensitive",
			exe:         `C:\Users\dev\Scoop\Apps\jira\current\jira.exe`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelScoop,
		},
		{
			name:        "brew cellar path wins over goreleaser stamp",
			exe:         `/usr/local/Cellar/jira/0.6.5/bin/jira`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelBrew,
		},
		{
			name:        "brew prefix path (macOS ARM)",
			exe:         `/opt/homebrew/bin/jira`,
			mainVersion: "(devel)",
			buildBy:     "unknown",
			vcsStamped:  true,
			want:        ChannelBrew,
		},
		{
			name:        "linuxbrew path",
			exe:         `/home/linuxbrew/.linuxbrew/Cellar/jira/0.6.5/bin/jira`,
			mainVersion: "(devel)",
			buildBy:     "unknown",
			vcsStamped:  true,
			want:        ChannelBrew,
		},
		{
			name:        "mise tool install wins over goreleaser stamp",
			exe:         `/home/dev/.local/share/mise/installs/github-matcra587-jira-cli/0.6.5/jira`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelMise,
		},
		{
			name:        "mise Go toolchain GOBIN is not a mise install",
			exe:         `C:\Users\dev\AppData\Local\mise\installs\go\1.26.4\bin\jira.exe`,
			mainVersion: "v0.6.5",
			buildBy:     "unknown",
			vcsStamped:  false,
			want:        ChannelGoInstall,
		},
		{
			name:        "module proxy build",
			exe:         `/home/dev/go/bin/jira`,
			mainVersion: "v0.6.5",
			buildBy:     "unknown",
			vcsStamped:  false,
			want:        ChannelGoInstall,
		},
		{
			name:        "pseudo-version module proxy build",
			exe:         `/home/dev/go/bin/jira`,
			mainVersion: "v0.6.6-0.20260701120000-abcdef123456",
			buildBy:     "unknown",
			vcsStamped:  false,
			want:        ChannelGoInstall,
		},
		{
			name:        "source build with VCS-derived version is not go-install",
			exe:         `/home/dev/src/jira-cli/jira`,
			mainVersion: "v0.6.5-0.20260703040000-abcdef123456",
			buildBy:     "unknown",
			vcsStamped:  true,
			want:        ChannelUnknown,
		},
		{
			name:        "goreleaser archive install",
			exe:         `/home/dev/.local/bin/jira`,
			mainVersion: "(devel)",
			buildBy:     "goreleaser",
			vcsStamped:  true,
			want:        ChannelArchive,
		},
		{
			name:        "plain go build is unknown",
			exe:         `/home/dev/src/jira-cli/jira`,
			mainVersion: "(devel)",
			buildBy:     "unknown",
			vcsStamped:  true,
			want:        ChannelUnknown,
		},
		{
			name:        "no build info at all is unknown",
			exe:         "",
			mainVersion: "",
			buildBy:     "unknown",
			vcsStamped:  false,
			want:        ChannelUnknown,
		},
		{
			name:        "developer path containing scoop as a word is not scoop",
			exe:         `/home/dev/projects/scoopberg/jira`,
			mainVersion: "(devel)",
			buildBy:     "unknown",
			vcsStamped:  true,
			want:        ChannelUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := detect(tt.exe, tt.mainVersion, tt.buildBy, tt.vcsStamped)
			if got != tt.want {
				t.Errorf("detect(%q, %q, %q, %v) = %q, want %q", tt.exe, tt.mainVersion, tt.buildBy, tt.vcsStamped, got, tt.want)
			}
		})
	}
}

func TestUpdateAvailable(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{"newer release", "v0.6.4", "v0.6.5", true},
		{"same release", "v0.6.5", "v0.6.5", false},
		{"older release", "v0.6.6", "v0.6.5", false},
		{"dev build behind latest tag", "v0.6.5-gf95ee20-dev", "v0.6.6", true},
		{"unparseable current", "not-a-version", "v0.6.5", false},
		{"empty latest", "v0.6.5", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := UpdateAvailable(tt.current, tt.latest); got != tt.want {
				t.Errorf("UpdateAvailable(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestSelfUpdates(t *testing.T) {
	wantTrue := map[Channel]bool{ChannelBrew: true, ChannelArchive: true}
	for _, ch := range []Channel{
		ChannelBrew, ChannelScoop, ChannelMise, ChannelGoInstall, ChannelArchive, ChannelUnknown,
	} {
		if got := ch.SelfUpdates(); got != wantTrue[ch] {
			t.Errorf("%s.SelfUpdates() = %v, want %v", ch, got, wantTrue[ch])
		}
	}
}

func TestNewUpdaterRefusesManagedChannels(t *testing.T) {
	for _, ch := range []Channel{ChannelScoop, ChannelMise, ChannelGoInstall, ChannelUnknown} {
		if _, err := NewUpdater(ch); err == nil {
			t.Errorf("NewUpdater(%s) = nil error, want does-not-self-update error", ch)
		}
	}
}
