package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Settings struct {
	IncludeList []string `json:"include_list"`
	BlackList   []string `json:"black_list"`

	Percent                   float64 `json:"percent"`
	PercentDurationDifference float64 `json:"percent_duration_difference"`
	DurationDifferenceMinSec  float64 `json:"duration_difference_min_seconds"`
	DurationDifferenceMaxSec  float64 `json:"duration_difference_max_seconds"`

	FilterByFileSize bool  `json:"filter_by_file_size"`
	MinimumFileSize  int64 `json:"minimum_file_size"`
	MaximumFileSize  int64 `json:"maximum_file_size"`

	Thumbnails  int `json:"thumbnails"`
	Concurrency int `json:"concurrency"`

	AutoFetchThumbnails bool `json:"auto_fetch_thumbnails"`
	RecheckSuspicious   bool `json:"recheck_suspicious"`

	// UI Display Settings
	ShowMediaInfo  bool `json:"show_media_info"`
	ShowSimilarity bool `json:"show_similarity"`
	ShowThumbnails bool `json:"show_thumbnails"`

	DebugLogging bool `json:"debug_logging"`
}

type SettingsManager struct {
	mu       sync.RWMutex
	settings Settings
	filePath string
}

func NewSettingsManager(filePath string) *SettingsManager {
	sm := &SettingsManager{
		filePath: filePath,
		settings: Settings{
			Percent:                   96.0,
			PercentDurationDifference: 20.0,
			Thumbnails:                4,
			Concurrency:               4,
			AutoFetchThumbnails:       true,
			RecheckSuspicious:         false,
			ShowMediaInfo:             true,
			ShowSimilarity:            true,
			ShowThumbnails:            true,
		},
	}
	sm.Load()
	return sm
}

func (sm *SettingsManager) Load() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	data, err := os.ReadFile(sm.filePath)
	if err == nil {
		_ = json.Unmarshal(data, &sm.settings)
	}
}

func (sm *SettingsManager) Save() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	data, err := json.MarshalIndent(sm.settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(sm.filePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(sm.filePath, data, 0644)
}

func (sm *SettingsManager) Get() Settings {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.settings
}

func (sm *SettingsManager) Update(s Settings) error {
	sm.mu.Lock()
	sm.settings = s
	sm.mu.Unlock()
	return sm.Save()
}

func (sm *SettingsManager) Reset() error {
	defaults := Settings{
		Percent:                   96.0,
		PercentDurationDifference: 20.0,
		Thumbnails:                4,
		Concurrency:               4,
		AutoFetchThumbnails:       true,
		RecheckSuspicious:         false,
		ShowMediaInfo:             true,
		ShowSimilarity:            true,
		ShowThumbnails:            true,
	}
	return sm.Update(defaults)
}
