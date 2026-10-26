// queue.go — مدير الطوابير: حالات، سرعات حية، سجل أحداث، استمرارية الواجهة.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type TaskState string

const (
	StateQueued   TaskState = "queued"
	StateRunning  TaskState = "running"
	StateDone     TaskState = "done"
	StateFailed   TaskState = "failed"
	StateCanceled TaskState = "canceled"
)

// Task مهمة تنزيل مع تقدمها الحي.
type Task struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Filename  string    `json:"filename"`
	Kind      string    `json:"kind"`
	Platform  string    `json:"platform"`
	Dir       string    `json:"dir"`
	Segments  int       `json:"segments"`
	State     TaskState `json:"state"`
	Done      int64     `json:"done"`
	Total     int64     `json:"total"`
	SpeedBps  int64     `json:"speed_bps"`
	Err       string    `json:"err,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LogEvent struct {
	TS    time.Time `json:"ts"`
	Level string    `json:"level"` // info|error|success
	Text  string    `json:"text"`
}

type Config struct {
	MaxParallel int
	DownloadDir string
	Client      *http.Client
}

type speedSample struct {
	last int64
	at   time.Time
}

type Manager struct {
	mu       sync.Mutex
	tasks    map[string]*Task
	order    []string
	cancels  map[string]context.CancelFunc
	canceled map[string]bool
	samples  map[string]*speedSample
	events   []LogEvent
	cfg      Config
	tools    Tools
	persist  func(tasks []Task)
}

func NewManager(cfg Config, tools Tools, persist func([]Task)) *Manager {
	if cfg.MaxParallel <= 0 {
		cfg.MaxParallel = 3
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = "./downloads"
	}
	return &Manager{
		tasks:    make(map[string]*Task),
		cancels:  make(map[string]context.CancelFunc),
		canceled: make(map[string]bool),
		samples:  make(map[string]*speedSample),
		cfg:      cfg,
		tools:    tools,
		persist:  persist,
	}
}

// Restore يستعيد المهام من القرص — التشغيلة تنقسم إلى queued (استكمال v0.2).
func (m *Manager) Restore(ts []Task) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range ts {
		t := ts[i]
		if t.State == StateRunning || t.State == StateQueued {
			t.State = StateQueued
		}
		m.tasks[t.ID] = &t
		m.order = append(m.order, t.ID)
	}
	if len(ts) > 0 {
		m.log("info", fmt.Sprintf("استُعيدة %d مهمة من القرص (الاستكمال الجزئي في v0.2)", len(ts)))
	}
}

func (m *Manager) log(level, text string) {
	m.events = append(m.events, LogEvent{TS: time.Now().UTC(), Level: level, Text: text})
	if len(m.events) > 800 {
		m.events = m.events[len(m.events)-800:]
	}
}

func (m *Manager) snapshot() []Task {
	out := make([]Task, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, *m.tasks[id])
	}
	return out
}

func (m *Manager) persistLocked() {
	if m.persist != nil {
		m.persist(m.snapshot())
	}
}

// resolveDir يقفل مجلد الوجهة داخل شجرة مجلد التنزيلات (أمان).
func (m *Manager) resolveDir(req string) (string, error) {
	base, err := filepath.Abs(m.cfg.DownloadDir)
	if err != nil {
		return "", err
	}
	if req == "" {
		return base, nil
	}
	abs, err := filepath.Abs(req)
	if err != nil {
		return "", err
	}
	if abs != base && !strings.HasPrefix(abs, base+string(filepath.Separator)) {
		return "", fmt.Errorf("المجلد خارج شجرة التنزيلات — مرفوض (أمان)")
	}
	_ = os.MkdirAll(abs, 0o755)
	return abs, nil
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "t" + hex.EncodeToString(b) + fmt.Sprintf("%04x", time.Now().UnixMilli()&0xFFFF)
}

// AnalyzeOnly تحليل دون إضافة للطابور.
func (m *Manager) AnalyzeOnly(urlStr string) (*AnalyzedFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return Analyze(ctx, m.cfg.Client, urlStr, m.tools)
}

// Add يحلل ويضيف للطابور ويشغّل إن توفرت فتحة.
func (m *Manager) Add(urlStr, dir string, segs int) (*Task, error) {
	an, err := m.AnalyzeOnly(urlStr)
	if err != nil {
		return nil, err
	}
	okDir, err := m.resolveDir(dir)
	if err != nil {
		return nil, err
	}
	if an.Kind == "torrent" || an.Platform != "" {
		return nil, fmt.Errorf("v0.1 ينفّذ ملفات http مباشرة فقط — المنصات/التورنت قيد الجسر (v0.2): %s", an.Platform)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	t := &Task{
		ID: newID(), URL: an.URL, Filename: an.Filename, Kind: an.Kind,
		Platform: an.Platform, Dir: okDir, Segments: segs,
		State: StateQueued, Total: an.SizeBytes,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.tasks[t.ID] = t
	m.order = append(m.order, t.ID)
	m.log("info", fmt.Sprintf("مهمة جديدة: %s (%s · %s)", t.Filename, t.Kind, humanBytes(t.Total)))
	m.persistLocked()
	m.kickLocked()
	return t, nil
}

// Kick يشغّل المهام المنتظرة (نقطة الدخول العلنية للاستعادة من القرص).
func (m *Manager) Kick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kickLocked()
}

func (m *Manager) kickLocked() {
	for _, id := range m.order {
		if len(m.cancels) >= m.cfg.MaxParallel {
			return
		}
		t := m.tasks[id]
		if t == nil || t.State != StateQueued {
			continue
		}
		m.startLocked(t)
	}
}

func (m *Manager) startLocked(t *Task) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[t.ID] = cancel
	t.State = StateRunning
	t.Err = ""
	t.UpdatedAt = time.Now().UTC()
	m.samples[t.ID] = &speedSample{at: time.Now().UTC(), last: 0}
	m.log("info", fmt.Sprintf("بدأ التحميل: %s (%d جزء)", t.Filename, t.Segments))
	m.persistLocked()

	out := filepath.Join(t.Dir, t.Filename)
	id := t.ID
	segments := t.Segments
	go func() {
		done, err := ParallelDownload(ctx, m.cfg.Client, m.tasksURL(id), out, segments, func(d, tt int64) {
			m.progress(id, d, tt)
		})
		m.finish(id, done, err)
	}()
}

// tasksURL يقرأ الرابط تحت القفل (بطلون مسابقتي القراءة).
func (m *Manager) tasksURL(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if t := m.tasks[id]; t != nil {
		return t.URL
	}
	return ""
}

func (m *Manager) progress(id string, done, total int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return
	}
	t.Done = done
	if total > 0 {
		t.Total = total
	}
	now := time.Now().UTC()
	if s, ok := m.samples[id]; ok {
		if dt := now.Sub(s.at).Seconds(); dt > 0.3 {
			inst := float64(done-s.last) / dt
			t.SpeedBps = int64(0.6*float64(t.SpeedBps) + 0.4*inst)
			s.last = done
			s.at = now
		}
	}
}

func (m *Manager) finish(id string, done int64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := m.tasks[id]
	if t == nil {
		return
	}
	t.Done = done
	t.UpdatedAt = time.Now().UTC()
	delete(m.samples, id)
	switch {
	case m.canceled[id] && err != nil:
		t.State = StateCanceled
		t.Err = "إيقاف/إلغاء (الجزئي محفوظ — الاستكمال في v0.2)"
		t.SpeedBps = 0
	case err != nil:
		t.State = StateFailed
		t.Err = err.Error()
		m.log("error", "فشل: "+t.Filename+" — "+t.Err)
	default:
		t.State = StateDone
		t.SpeedBps = 0
		m.log("success", "اكتمل: "+t.Filename+" ("+humanBytes(done)+")")
	}
	if cf := m.cancels[id]; cf != nil {
		cf()
		delete(m.cancels, id)
	}
	m.persistLocked()
	m.kickLocked()
}

// Pause يوقف المهمة (الجزئي محفوظ) — الاستكمال الحقيقي في v0.2.
func (m *Manager) Pause(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cf, ok := m.cancels[id]; ok {
		m.canceled[id] = true
		cf()
		m.log("info", "أوقف مؤقتًا: "+m.tasks[id].Filename)
		return true
	}
	if t := m.tasks[id]; t != nil && t.State == StateQueued {
		t.State = StateCanceled
		t.Err = "أُلغيت قبل البدء"
		m.persistLocked()
		return true
	}
	return false
}

// List يعيد قائمة المهام مرتبة من الأحدث.
func (m *Manager) List() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Task, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		out = append(out, *m.tasks[m.order[i]])
	}
	return out
}

// History المكتملة والفاشلة (الأحدث أولًا).
func (m *Manager) History() []Task {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Task
	for i := len(m.order) - 1; i >= 0; i-- {
		t := *m.tasks[m.order[i]]
		if t.State == StateDone || t.State == StateFailed || t.State == StateCanceled {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// Events يعيد الأحداث حسب التبويب: all|errors|success.
func (m *Manager) Events(tab string) []LogEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []LogEvent
	for _, e := range m.events {
		switch tab {
		case "errors":
			if e.Level == "error" {
				out = append(out, e)
			}
		case "success":
			if e.Level == "success" {
				out = append(out, e)
			}
		default:
			out = append(out, e)
		}
	}
	if len(out) > 200 {
		out = out[len(out)-200:]
	}
	return out
}

// Stats ملخص حي للوحة الرئيسية.
func (m *Manager) Stats(storageFn func() int64) map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := map[string]any{
		"active": 0, "done": 0, "failed": 0, "down_bps": 0, "up_bps": 0,
		"types": map[string]int{},
	}
	types := st["types"].(map[string]int)
	var down int64
	for i := len(m.order) - 1; i >= 0; i-- {
		t := *m.tasks[m.order[i]]
		switch t.State {
		case StateRunning:
			st["active"] = st["active"].(int) + 1
			down += t.SpeedBps
		case StateDone:
			st["done"] = st["done"].(int) + 1
		case StateFailed:
			st["failed"] = st["failed"].(int) + 1
		}
		if t.Filename != "" {
			ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(t.Filename)), ".")
			if ext == "" {
				ext = "other"
			}
			types[ext]++
		}
	}
	st["down_bps"] = down
	st["max_parallel"] = m.cfg.MaxParallel
	st["download_dir"] = m.cfg.DownloadDir
	if storageFn != nil {
		st["storage_bytes"] = storageFn()
	}
	return st
}

func humanBytes(n int64) string {
	switch {
	case n <= 0:
		return "0 B"
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
	}
}
