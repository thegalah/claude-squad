package git

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// PRState is the state of a pull request, as the session list shows it.
type PRState string

const (
	PROpen   PRState = "open"
	PRDraft  PRState = "draft"
	PRMerged PRState = "merged"
	PRClosed PRState = "closed"
)

// PullRequest is the GitHub pull request opened from a session's branch.
type PullRequest struct {
	Number int     `json:"number"`
	State  PRState `json:"state"`
	URL    string  `json:"url"`
}

// BranchRef is a session's branch in a local repository, to look up the pull request for.
type BranchRef struct {
	RepoPath string
	Branch   string
}

// GHAvailable reports whether the GitHub CLI is installed, which looking up pull requests
// needs.
func GHAvailable() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// ghGraphQL runs a GraphQL query against host's API with gh and returns the response.
// Tests replace it.
var ghGraphQL = func(host string, args []string) ([]byte, error) {
	cmd := exec.Command("gh", append([]string{"api", "graphql", "--hostname", host}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("gh api graphql: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("gh api graphql: %w", err)
	}
	return out, nil
}

// ghRepo is a repository on a GitHub host.
type ghRepo struct {
	host, owner, name string
}

func (r ghRepo) nameWithOwner() string { return r.owner + "/" + r.name }

// FindPullRequests looks up the pull request for each branch, in the GitHub repository the
// branch is pushed to (its push remote, as `git push` would pick it), and in that
// repository's parent if it's a fork, for pull requests from the fork into upstream. A
// branch with several pull requests gets its open one, or else its newest.
//
// The result has an entry for every branch whose lookup succeeded, nil when it has no pull
// request; branches whose lookup failed are left out, so callers can keep what they knew.
// There's one query per GitHub repository, however many branches it has.
func FindPullRequests(refs []BranchRef) (map[BranchRef]*PullRequest, error) {
	found := make(map[BranchRef]*PullRequest)
	var errs []error

	byRepo := make(map[ghRepo][]BranchRef)
	byPath := make(map[string][]BranchRef)
	for _, ref := range refs {
		byPath[ref.RepoPath] = append(byPath[ref.RepoPath], ref)
	}
	for path, pathRefs := range byPath {
		remotes, err := pushRemotes(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		urls := make(map[string]*ghRepo)
		for _, ref := range pathRefs {
			remote := remotes.forBranch(ref.Branch)
			repo, ok := urls[remote]
			if !ok {
				repo = remoteRepo(path, remote)
				urls[remote] = repo
			}
			if repo == nil {
				// Not pushed anywhere on GitHub, so there's no pull request to find.
				found[ref] = nil
				continue
			}
			byRepo[*repo] = append(byRepo[*repo], ref)
		}
	}

	for repo, repoRefs := range byRepo {
		var branches []string
		seen := make(map[string]bool)
		for _, ref := range repoRefs {
			if !seen[ref.Branch] {
				seen[ref.Branch] = true
				branches = append(branches, ref.Branch)
			}
		}
		prs, err := queryPullRequests(repo, branches)
		if err != nil {
			errs = append(errs, fmt.Errorf("could not look up pull requests in %s: %w", repo.nameWithOwner(), err))
			continue
		}
		for _, ref := range repoRefs {
			found[ref] = prs[ref.Branch]
		}
	}
	return found, errors.Join(errs...)
}

// remoteConfig is the git config that decides which remote each branch pushes to.
type remoteConfig struct {
	pushDefault string
	pushRemote  map[string]string // branch.<name>.pushRemote
	remote      map[string]string // branch.<name>.remote
}

// forBranch returns the remote `git push` would push branch to.
func (c remoteConfig) forBranch(branch string) string {
	for _, r := range []string{c.pushRemote[branch], c.pushDefault, c.remote[branch]} {
		if r != "" {
			return r
		}
	}
	return "origin"
}

// pushRemotes reads the push remote settings of the repository at path.
func pushRemotes(path string) (remoteConfig, error) {
	c := remoteConfig{pushRemote: map[string]string{}, remote: map[string]string{}}
	out, err := exec.Command("git", "-C", path, "config", "--get-regexp",
		`^(branch\..+\.(remote|pushremote)|remote\.pushdefault)$`).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return c, nil // none set
		}
		return c, fmt.Errorf("could not read git config in %s: %w", path, err)
	}
	parseRemoteConfig(string(out), &c)
	return c, nil
}

// parseRemoteConfig parses `git config --get-regexp` output into c. Git lowercases section
// and variable names, but not branch names.
func parseRemoteConfig(out string, c *remoteConfig) {
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch {
		case key == "remote.pushdefault":
			c.pushDefault = value
		case strings.HasPrefix(key, "branch.") && strings.HasSuffix(key, ".pushremote"):
			c.pushRemote[strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".pushremote")] = value
		case strings.HasPrefix(key, "branch.") && strings.HasSuffix(key, ".remote"):
			c.remote[strings.TrimSuffix(strings.TrimPrefix(key, "branch."), ".remote")] = value
		}
	}
}

// remoteRepo returns the GitHub repository remote points to in the repository at path, or
// nil if it isn't one (or doesn't exist).
func remoteRepo(path, remote string) *ghRepo {
	if remote == "." {
		return nil // pushes to the local repository
	}
	out, err := exec.Command("git", "-C", path, "remote", "get-url", "--push", remote).Output()
	if err != nil {
		return nil
	}
	repo, ok := parseGitHubRemote(strings.TrimSpace(string(out)))
	if !ok {
		return nil
	}
	return &repo
}

// parseGitHubRemote parses a remote URL (https, ssh, git, or scp-like user@host:path)
// into the repository it points to. Local paths aren't repositories on a host. A host
// without a dot is taken to be an ssh alias for github.com.
func parseGitHubRemote(remote string) (ghRepo, bool) {
	var host, path string
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil || u.Scheme == "file" {
			return ghRepo{}, false
		}
		host, path = u.Hostname(), u.Path
	} else {
		// scp-like: [user@]host:owner/name. A colon after a slash means a local path.
		h, p, ok := strings.Cut(remote, ":")
		if !ok || strings.Contains(h, "/") {
			return ghRepo{}, false
		}
		if at := strings.LastIndex(h, "@"); at >= 0 {
			h = h[at+1:]
		}
		host, path = h, p
	}
	if host == "" {
		return ghRepo{}, false
	}
	if !strings.Contains(host, ".") && host != "localhost" {
		host = "github.com"
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ghRepo{}, false
	}
	name := strings.TrimSuffix(parts[1], ".git")
	if name == "" {
		return ghRepo{}, false
	}
	return ghRepo{host: strings.ToLower(host), owner: parts[0], name: name}, true
}

// maxBranchesPerQuery caps how many branches one GraphQL query asks about.
const maxBranchesPerQuery = 50

// queryPullRequests looks up the pull request for each of branches in repo (and its
// parent, if it's a fork), keyed by branch. Branches without one are left out.
func queryPullRequests(repo ghRepo, branches []string) (map[string]*PullRequest, error) {
	prs := make(map[string]*PullRequest)
	for start := 0; start < len(branches); start += maxBranchesPerQuery {
		chunk := branches[start:min(start+maxBranchesPerQuery, len(branches))]
		args := []string{"-f", "query=" + prQuery(len(chunk)), "-f", "owner=" + repo.owner, "-f", "name=" + repo.name}
		for i, b := range chunk {
			args = append(args, "-f", fmt.Sprintf("b%d=%s", i, b))
		}
		out, err := ghGraphQL(repo.host, args)
		if err != nil {
			return nil, err
		}
		chunkPRs, err := parsePRResponse(out, repo, len(chunk))
		if err != nil {
			return nil, err
		}
		for i, pr := range chunkPRs {
			if pr != nil {
				prs[chunk[i]] = pr
			}
		}
	}
	return prs, nil
}

// prQuery is a GraphQL query for the pull requests from n branches ($b0, $b1, ...) in
// repository $owner/$name and its parent.
func prQuery(n int) string {
	var vars, fields strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&vars, ",$b%d:String!", i)
		fmt.Fprintf(&fields, " b%d:pullRequests(headRefName:$b%d,first:10,orderBy:{field:CREATED_AT,direction:DESC}){nodes{...pr}}", i, i)
	}
	return fmt.Sprintf("query($owner:String!,$name:String!%s){repository(owner:$owner,name:$name){%s parent{%s}}}"+
		" fragment pr on PullRequest{number state isDraft url createdAt headRepository{nameWithOwner}}",
		vars.String(), fields.String(), fields.String())
}

type prNode struct {
	Number         int       `json:"number"`
	State          string    `json:"state"`
	IsDraft        bool      `json:"isDraft"`
	URL            string    `json:"url"`
	CreatedAt      time.Time `json:"createdAt"`
	HeadRepository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"headRepository"`
}

type prConnection struct {
	Nodes []prNode `json:"nodes"`
}

// parsePRResponse picks the pull request for each of n queried branches from a prQuery
// response. Only pull requests whose head is the branch in repo count: in the parent,
// other forks may have branches with the same name.
func parsePRResponse(out []byte, repo ghRepo, n int) ([]*PullRequest, error) {
	var resp struct {
		Data struct {
			Repository map[string]json.RawMessage `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("could not parse response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, errors.New(resp.Errors[0].Message)
	}
	if resp.Data.Repository == nil {
		return nil, fmt.Errorf("repository %s not found", repo.nameWithOwner())
	}

	conns := []map[string]json.RawMessage{resp.Data.Repository}
	if raw := resp.Data.Repository["parent"]; len(raw) > 0 && string(raw) != "null" {
		var parent map[string]json.RawMessage
		if err := json.Unmarshal(raw, &parent); err != nil {
			return nil, fmt.Errorf("could not parse response: %w", err)
		}
		conns = append(conns, parent)
	}

	prs := make([]*PullRequest, n)
	for i := range prs {
		var nodes []prNode
		for _, c := range conns {
			var conn prConnection
			if raw, ok := c[fmt.Sprintf("b%d", i)]; ok {
				if err := json.Unmarshal(raw, &conn); err != nil {
					return nil, fmt.Errorf("could not parse response: %w", err)
				}
			}
			for _, node := range conn.Nodes {
				if node.HeadRepository != nil && strings.EqualFold(node.HeadRepository.NameWithOwner, repo.nameWithOwner()) {
					nodes = append(nodes, node)
				}
			}
		}
		prs[i] = pickPullRequest(nodes)
	}
	return prs, nil
}

// pickPullRequest returns the newest open pull request among nodes, or else the newest.
func pickPullRequest(nodes []prNode) *PullRequest {
	if len(nodes) == 0 {
		return nil
	}
	sort.SliceStable(nodes, func(a, b int) bool {
		aOpen, bOpen := nodes[a].State == "OPEN", nodes[b].State == "OPEN"
		if aOpen != bOpen {
			return aOpen
		}
		return nodes[a].CreatedAt.After(nodes[b].CreatedAt)
	})
	n := nodes[0]
	pr := &PullRequest{Number: n.Number, URL: n.URL}
	switch {
	case n.State == "OPEN" && n.IsDraft:
		pr.State = PRDraft
	case n.State == "OPEN":
		pr.State = PROpen
	case n.State == "MERGED":
		pr.State = PRMerged
	default:
		pr.State = PRClosed
	}
	return pr
}
