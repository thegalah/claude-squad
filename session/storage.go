package session

import (
	"claude-squad/config"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	Branch    string    `json:"branch"`
	Status    Status    `json:"status"`
	Height    int       `json:"height"`
	Width     int       `json:"width"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	AutoYes   bool      `json:"auto_yes"`

	Program   string          `json:"program"`
	Worktree  GitWorktreeData `json:"worktree"`
	DiffStats DiffStatsData   `json:"diff_stats"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath         string `json:"repo_path"`
	WorktreePath     string `json:"worktree_path"`
	SessionName      string `json:"session_name"`
	BranchName       string `json:"branch_name"`
	BaseCommitSHA    string `json:"base_commit_sha"`
	IsExistingBranch bool   `json:"is_existing_branch"`
}

// DiffStatsData represents the serializable data of a DiffStats
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"content"`
}

// Storage handles saving and loading instances using the state interface
type Storage struct {
	state config.InstanceStorage
	// repoRoot, when non-empty, scopes this storage to instances belonging to that git
	// repository. Instances from other repositories are neither loaded nor overwritten.
	repoRoot string
}

// NewStorage creates a new storage instance that sees every stored instance
func NewStorage(state config.InstanceStorage) (*Storage, error) {
	return &Storage{
		state: state,
	}, nil
}

// NewRepoStorage creates a storage instance scoped to the git repository at repoRoot,
// so that cs processes running in different repositories don't see each other's sessions.
func NewRepoStorage(state config.InstanceStorage, repoRoot string) (*Storage, error) {
	if repoRoot == "" {
		return nil, fmt.Errorf("repo root must not be empty")
	}
	return &Storage{
		state:    state,
		repoRoot: filepath.Clean(repoRoot),
	}, nil
}

// instanceScope is the subset of InstanceData needed to decide which repository an
// instance belongs to.
type instanceScope struct {
	Path     string `json:"path"`
	Worktree struct {
		RepoPath string `json:"repo_path"`
	} `json:"worktree"`
}

func (sc instanceScope) repo() string {
	if sc.Worktree.RepoPath != "" {
		return sc.Worktree.RepoPath
	}
	return sc.Path
}

// inScope reports whether an instance stored under repo belongs to this storage
func (s *Storage) inScope(repo string) bool {
	if s.repoRoot == "" {
		return true
	}
	return repo != "" && filepath.Clean(repo) == s.repoRoot
}

// SaveInstances saves the list of instances to disk. For repo-scoped storage, only this
// repository's instances are replaced; instances from other repositories are preserved.
func (s *Storage) SaveInstances(instances []*Instance) error {
	// Convert instances to InstanceData
	data := make([]InstanceData, 0)
	for _, instance := range instances {
		if instance.Started() {
			data = append(data, instance.ToInstanceData())
		}
	}

	if s.repoRoot == "" {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return fmt.Errorf("failed to marshal instances: %w", err)
		}
		return s.state.SaveInstances(jsonData)
	}

	return s.state.UpdateInstances(func(current json.RawMessage) (json.RawMessage, error) {
		var existing []json.RawMessage
		if err := json.Unmarshal(current, &existing); err != nil {
			return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
		}
		merged := make([]json.RawMessage, 0, len(existing)+len(data))
		for _, raw := range existing {
			var sc instanceScope
			if err := json.Unmarshal(raw, &sc); err != nil {
				return nil, fmt.Errorf("failed to unmarshal instance: %w", err)
			}
			if !s.inScope(sc.repo()) {
				merged = append(merged, raw)
			}
		}
		for _, d := range data {
			raw, err := json.Marshal(d)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal instance %s: %w", d.Title, err)
			}
			merged = append(merged, raw)
		}
		return json.Marshal(merged)
	})
}

// LoadInstances loads the list of instances from disk
func (s *Storage) LoadInstances() ([]*Instance, error) {
	jsonData := s.state.GetInstances()

	var instancesData []InstanceData
	if err := json.Unmarshal(jsonData, &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}

	instances := make([]*Instance, 0, len(instancesData))
	for _, data := range instancesData {
		sc := instanceScope{Path: data.Path}
		sc.Worktree.RepoPath = data.Worktree.RepoPath
		if !s.inScope(sc.repo()) {
			continue
		}
		instance, err := FromInstanceData(data)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %s: %w", data.Title, err)
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// DeleteInstance removes an instance from storage
func (s *Storage) DeleteInstance(title string) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	found := false
	newInstances := make([]*Instance, 0)
	for _, instance := range instances {
		data := instance.ToInstanceData()
		if data.Title != title {
			newInstances = append(newInstances, instance)
		} else {
			found = true
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", title)
	}

	return s.SaveInstances(newInstances)
}

// UpdateInstance updates an existing instance in storage
func (s *Storage) UpdateInstance(instance *Instance) error {
	instances, err := s.LoadInstances()
	if err != nil {
		return fmt.Errorf("failed to load instances: %w", err)
	}

	data := instance.ToInstanceData()
	found := false
	for i, existing := range instances {
		existingData := existing.ToInstanceData()
		if existingData.Title == data.Title {
			instances[i] = instance
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("instance not found: %s", data.Title)
	}

	return s.SaveInstances(instances)
}

// DeleteAllInstances removes all stored instances
func (s *Storage) DeleteAllInstances() error {
	return s.state.DeleteAllInstances()
}

// TrackedWorktreePaths returns the worktree paths of every stored instance, across all
// repositories, without restoring the instances.
func TrackedWorktreePaths(state config.InstanceStorage) ([]string, error) {
	var instancesData []InstanceData
	if err := json.Unmarshal(state.GetInstances(), &instancesData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal instances: %w", err)
	}
	paths := make([]string, 0, len(instancesData))
	for _, data := range instancesData {
		if data.Worktree.WorktreePath != "" {
			paths = append(paths, data.Worktree.WorktreePath)
		}
	}
	return paths, nil
}
