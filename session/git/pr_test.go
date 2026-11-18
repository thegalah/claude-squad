package git

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseGitHubRemote(t *testing.T) {
	for _, tc := range []struct {
		url  string
		want ghRepo
		ok   bool
	}{
		{"https://github.com/thegalah/claude-squad.git", ghRepo{"github.com", "thegalah", "claude-squad"}, true},
		{"https://github.com/thegalah/claude-squad", ghRepo{"github.com", "thegalah", "claude-squad"}, true},
		{"https://user:token@github.com/thegalah/claude-squad/", ghRepo{"github.com", "thegalah", "claude-squad"}, true},
		{"git@github.com:smtg-ai/claude-squad.git", ghRepo{"github.com", "smtg-ai", "claude-squad"}, true},
		{"ssh://git@github.com:22/smtg-ai/claude-squad.git", ghRepo{"github.com", "smtg-ai", "claude-squad"}, true},
		{"git@github-work:acme/app.git", ghRepo{"github.com", "acme", "app"}, true}, // ssh alias
		{"https://ghe.example.com/acme/app.git", ghRepo{"ghe.example.com", "acme", "app"}, true},
		{"/home/me/repos/app", ghRepo{}, false},
		{"../app", ghRepo{}, false},
		{"file:///home/me/app.git", ghRepo{}, false},
		{"https://gitlab.com/group/sub/app.git", ghRepo{}, false},
	} {
		got, ok := parseGitHubRemote(tc.url)
		require.Equal(t, tc.ok, ok, tc.url)
		require.Equal(t, tc.want, got, tc.url)
	}
}

func TestRemoteConfigPicksPushRemoteLikeGit(t *testing.T) {
	var c remoteConfig
	c.pushRemote, c.remote = map[string]string{}, map[string]string{}
	parseRemoteConfig(strings.Join([]string{
		"branch.main.remote origin",
		"branch.feat/a.b.remote upstream",
		"branch.feat/a.b.pushremote fork",
		"branch.Fix/Case.remote mine",
	}, "\n"), &c)

	require.Equal(t, "fork", c.forBranch("feat/a.b"), "pushRemote beats remote")
	require.Equal(t, "mine", c.forBranch("Fix/Case"), "branch names keep their case")
	require.Equal(t, "origin", c.forBranch("never-pushed"))

	c.pushDefault = "fork"
	require.Equal(t, "fork", c.forBranch("main"), "remote.pushDefault beats branch.<name>.remote")
}

func TestParsePRResponsePicksOwnBranchAndPrefersOpen(t *testing.T) {
	repo := ghRepo{"github.com", "me", "app"}
	out := `{"data":{"repository":{
		"b0":{"nodes":[
			{"number":7,"state":"CLOSED","isDraft":false,"url":"u7","createdAt":"2026-11-10T00:00:00Z","headRepository":{"nameWithOwner":"me/app"}},
			{"number":5,"state":"OPEN","isDraft":true,"url":"u5","createdAt":"2026-11-01T00:00:00Z","headRepository":{"nameWithOwner":"me/app"}}
		]},
		"b1":{"nodes":[]},
		"b2":{"nodes":[{"number":3,"state":"MERGED","isDraft":false,"url":"u3","createdAt":"2026-10-01T00:00:00Z","headRepository":{"nameWithOwner":"me/app"}}]},
		"parent":{
			"b0":{"nodes":[]},
			"b1":{"nodes":[
				{"number":90,"state":"OPEN","isDraft":false,"url":"u90","createdAt":"2026-11-12T00:00:00Z","headRepository":{"nameWithOwner":"someone-else/app"}},
				{"number":80,"state":"OPEN","isDraft":false,"url":"u80","createdAt":"2026-11-11T00:00:00Z","headRepository":{"nameWithOwner":"Me/App"}}
			]},
			"b2":{"nodes":[{"number":99,"state":"CLOSED","isDraft":false,"url":"u99","createdAt":"2026-11-15T00:00:00Z","headRepository":null}]}
		}
	}}}`
	prs, err := parsePRResponse([]byte(out), repo, 3)
	require.NoError(t, err)
	require.Equal(t, &PullRequest{Number: 5, State: PRDraft, URL: "u5"}, prs[0], "an open PR beats a newer closed one")
	require.Equal(t, &PullRequest{Number: 80, State: PROpen, URL: "u80"}, prs[1], "upstream PRs count only from this fork")
	require.Equal(t, &PullRequest{Number: 3, State: PRMerged, URL: "u3"}, prs[2], "PRs whose head repo is gone don't count")
}

func TestParsePRResponseErrors(t *testing.T) {
	repo := ghRepo{"github.com", "me", "app"}
	_, err := parsePRResponse([]byte(`{"data":{"repository":null},"errors":[{"message":"Could not resolve to a Repository"}]}`), repo, 1)
	require.ErrorContains(t, err, "Could not resolve")
	_, err = parsePRResponse([]byte(`{"data":{"repository":null}}`), repo, 1)
	require.Error(t, err)

	prs, err := parsePRResponse([]byte(`{"data":{"repository":{"b0":{"nodes":[]},"parent":null}}}`), repo, 1)
	require.NoError(t, err)
	require.Nil(t, prs[0])
}

// stubGH replaces gh for the test, and keeps the user's git config out of it.
func stubGH(t *testing.T, fn func(host string, args []string) ([]byte, error)) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
	orig := ghGraphQL
	ghGraphQL = fn
	t.Cleanup(func() { ghGraphQL = orig })
}

func TestFindPullRequestsQueriesEachBranchsPushRepoOnce(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "init", "-q", repo)
	mustRunGit(t, repo, "remote", "add", "origin", "git@github.com:me/app.git")
	mustRunGit(t, repo, "remote", "add", "upstream", "https://github.com/them/app.git")
	mustRunGit(t, repo, "config", "branch.to-upstream.pushRemote", "upstream")
	mustRunGit(t, repo, "config", "branch.local-only.pushRemote", ".")

	var queries []string
	stubGH(t, func(host string, args []string) ([]byte, error) {
		require.Equal(t, "github.com", host)
		vars := map[string]string{}
		for i := 0; i+1 < len(args); i += 2 {
			k, v, _ := strings.Cut(args[i+1], "=")
			vars[k] = v
		}
		queries = append(queries, vars["owner"]+"/"+vars["name"])
		var fields []string
		for i := 0; vars[fmt.Sprintf("b%d", i)] != ""; i++ {
			b := vars[fmt.Sprintf("b%d", i)]
			nodes := ""
			if b == "has-pr" || b == "to-upstream" {
				nodes = fmt.Sprintf(`{"number":%d,"state":"OPEN","isDraft":false,"url":"u","createdAt":"2026-11-01T00:00:00Z","headRepository":{"nameWithOwner":"%s/%s"}}`,
					len(b), vars["owner"], vars["name"])
			}
			fields = append(fields, fmt.Sprintf(`"b%d":{"nodes":[%s]}`, i, nodes))
		}
		return []byte(`{"data":{"repository":{` + strings.Join(fields, ",") + `,"parent":null}}}`), nil
	})

	hasPR := BranchRef{repo, "has-pr"}
	noPR := BranchRef{repo, "no-pr"}
	toUpstream := BranchRef{repo, "to-upstream"}
	localOnly := BranchRef{repo, "local-only"}
	found, err := FindPullRequests([]BranchRef{hasPR, noPR, toUpstream, localOnly, hasPR})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"me/app", "them/app"}, queries, "one query per GitHub repo")
	require.Equal(t, 6, found[hasPR].Number)
	require.Equal(t, 11, found[toUpstream].Number)
	require.Contains(t, found, noPR)
	require.Nil(t, found[noPR])
	require.Contains(t, found, localOnly)
	require.Nil(t, found[localOnly])
}

func TestFindPullRequestsLeavesOutFailedLookups(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	mustRunGit(t, "", "init", "-q", repo)
	mustRunGit(t, repo, "remote", "add", "origin", "git@github.com:me/app.git")
	stubGH(t, func(string, []string) ([]byte, error) { return nil, errors.New("gh: not logged in") })

	found, err := FindPullRequests([]BranchRef{{repo, "feat"}})
	require.ErrorContains(t, err, "not logged in")
	require.NotContains(t, found, BranchRef{repo, "feat"}, "so the caller keeps what it knew")
}
