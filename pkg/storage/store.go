// store.go — ملف حالات منظم (Settings/Tasks/History) بذري عبر tmp+rename.
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Settings إعدادات المنتج وحالة الترخيص (محلي فقط — لا اتصال خارجي).
type Settings struct {
	Product     string `json:"product"`
	Version     string `json:"version"`
	License     string `json:"license"` // pro-lifetime
	Owner       string `json:"owner"`
	Segments    int    `json:"segments"`
	DownloadDir string `json:"download_dir"`
	MaxParallel int    `json:"max_parallel"`
}

type Store struct {
	Dir      string
	Settings Settings
}

// التخزين ملف واحد: baranflow.state.json
func Default() Settings {
	return Settings{
		Product: "BaranFlow", Version: "0.1.0",
		License: "pro-lifetime", Owner: "Baran",
		Segments: 4, DownloadDir: "./downloads", MaxParallel: 3,
	}
}

func Load(dir string) (*Store, error) {
	if dir == "" {
		dir = "./data"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{Dir: dir, Settings: Default()}
	raw, err := os.ReadFile(filepath.Join(dir, "baranflow.state.json"))
	if err == nil {
		var snap struct {
			Settings Settings `json:"settings"`
		}
		if json.Unmarshal(raw, &snap) == nil && snap.Settings.Product != "" {
			s.Settings = snap.Settings
		}
	}
	return s, nil
}

// SaveSettings يكتب الإعدادات atomically.
func (s *Store) SaveSettings() error {
	f, err := os.CreateTemp(s.Dir, "state-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{"settings": s.Settings}); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, "baranflow.state.json"))
}

// SaveTasks يقرأ قائمة المهام وتقدمها من المدير ويخزنها — snapshot منفصل لفصل كموه العالم.
type TasksSnapshot struct {
	SavedAt string          `json:"saved_at"`
	Tasks   json.RawMessage `json:"tasks"`
}

func (s *Store) SaveTasks(tasksJSON []byte) error {
	f, err := os.CreateTemp(s.Dir, "tasks-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	snap := TasksSnapshot{SavedAt: fmt.Sprintf("%d", timeNow()), Tasks: json.RawMessage(tasksJSON)}
	enc := json.NewEncoder(f)
	if err := enc.Encode(snap); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, "tasks.json"))
}

func (s *Store) LoadTasks(dest any) error {
	raw, err := os.ReadFile(filepath.Join(s.Dir, "tasks.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var snap TasksSnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil // ملف تالف: نتجاهل مهذّبًا
	}
	return json.Unmarshal(snap.Tasks, dest)
}

func (s *Store) LoadSettingsJSON(dest any) error {
	raw, err := os.ReadFile(filepath.Join(s.Dir, "baranflow.state.json"))
	if err != nil {
		return err
	}
	var snap struct {
		Settings json.RawMessage `json:"settings"`
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return err
	}
	return json.Unmarshal(snap.Settings, dest)
}
