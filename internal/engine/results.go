package engine

import (
	"sort"
	"sync"
)

type ResultsManager struct {
	mu         sync.RWMutex
	results    []DuplicateGroup
	totalFiles int
}

type DuplicateGroup struct {
	ID    string     `json:"id"`
	Files []FileInfo `json:"files"`
}

type FileInfo struct {
	Path           string  `json:"path"`
	Size           int64   `json:"size"`
	Modified       int64   `json:"modified"`
	Duration       float64 `json:"duration"`
	Width          int     `json:"width"`
	Height         int     `json:"height"`
	Codec          string  `json:"codec"`
	Bitrate        int64   `json:"bitrate"`
	FPS            float64 `json:"fps"`
	Similarity     float64 `json:"similarity"`
	IdentifierHash string  `json:"identifier_hash"`
}

type ResultsResponse struct {
	Items      []DuplicateGroup `json:"items"`
	Total      int              `json:"total"`
	TotalFiles int              `json:"total_files"`
	Offset     int              `json:"offset"`
	Limit      int              `json:"limit"`
}

func NewResultsManager() *ResultsManager {
	return &ResultsManager{}
}

func (rm *ResultsManager) SetResults(results []DuplicateGroup) {
	// Sort results by similarity descending
	sort.Slice(results, func(i, j int) bool {
		avgI := getGroupSimilarity(results[i])
		avgJ := getGroupSimilarity(results[j])
		return avgI > avgJ
	})

	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.results = results
	rm.totalFiles = 0
	for _, g := range results {
		rm.totalFiles += len(g.Files)
	}
}

func getGroupSimilarity(g DuplicateGroup) float64 {
	if len(g.Files) == 0 {
		return 0
	}
	var sum float64
	var count int
	for _, f := range g.Files {
		if f.Similarity < 100 {
			sum += f.Similarity
			count++
		}
	}
	if count == 0 {
		return 100
	}
	return sum / float64(count)
}

func (rm *ResultsManager) GetResultsWrapped(offset, limit int) ResultsResponse {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalGroups := len(rm.results)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = totalGroups
	}

	start := min(offset, totalGroups)

	end := min(start+limit, totalGroups)

	items := rm.results[start:end]
	if items == nil {
		items = []DuplicateGroup{}
	}

	return ResultsResponse{
		Items:      items,
		Total:      totalGroups,
		TotalFiles: rm.totalFiles,
		Offset:     offset,
		Limit:      limit,
	}
}

func (rm *ResultsManager) GetResults(offset, limit int) ([]DuplicateGroup, int, int) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	totalGroups := len(rm.results)
	if offset >= totalGroups {
		return []DuplicateGroup{}, totalGroups, rm.totalFiles
	}

	end := min(offset+limit, totalGroups)

	return rm.results[offset:end], totalGroups, rm.totalFiles
}

func (rm *ResultsManager) RenameFile(oldPath, newPath string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for gi := range rm.results {
		for fi := range rm.results[gi].Files {
			if rm.results[gi].Files[fi].Path == oldPath {
				rm.results[gi].Files[fi].Path = newPath
			}
		}
	}
}

func (rm *ResultsManager) RemoveFiles(paths []string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	pathMap := make(map[string]bool)
	for _, p := range paths {
		pathMap[p] = true
	}

	var newResults []DuplicateGroup
	newTotalFiles := 0
	for _, group := range rm.results {
		var newFiles []FileInfo
		for _, file := range group.Files {
			if !pathMap[file.Path] {
				newFiles = append(newFiles, file)
			}
		}
		if len(newFiles) > 1 {
			group.Files = newFiles
			newResults = append(newResults, group)
			newTotalFiles += len(newFiles)
		}
	}
	rm.results = newResults
	rm.totalFiles = newTotalFiles
}

func (rm *ResultsManager) GetAll() ([]DuplicateGroup, int) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.results, rm.totalFiles
}

func (rm *ResultsManager) Clear() {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.results = nil
	rm.totalFiles = 0
}
