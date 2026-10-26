// external.go — كشف الأدوات الخارجية (صادق: يعلن وجودها أو غيابها) + جسر yt-dlp.
package engine

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Tools مسارات الأدوات الخارجية (فارغة = غير متوفرة).
type Tools struct {
	YtDlp   string
	FFmpeg  string
	Aria2C  string
	TRemote string // transmission-remote
}

// DetectTools يفحص PATH محليًا (سريع، بلا اتصال).
func DetectTools() Tools {
	return Tools{
		YtDlp:   look("yt-dlp"),
		FFmpeg:  look("ffmpeg"),
		Aria2C:  look("aria2c"),
		TRemote: look("transmission-remote"),
	}
}

func look(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// Capabilities أعلن الصدق — لا وهم للأدوات الغائبة.
func (t Tools) Capabilities() map[string]any {
	return map[string]any{
		"yt_dlp":         t.YtDlp != "",
		"ffmpeg":         t.FFmpeg != "",
		"aria2":          t.Aria2C != "",
		"torrent_bridge": t.Aria2C != "" || t.TRemote != "",
		"segments_max":   16,
		"parallel_tasks": 3,
		"honesty":        "الفيديو عبر yt-dlp إن وُجد — التورنت الكامل v0.2 عبر جسر خارجي — بلا ادعاءات وهمية",
	}
}

// RunYtDlpFormats يجلب قائمة الصيغ من yt-dlp (مهلة 25 ثانية).
func RunYtDlpFormats(ctx context.Context, path, urlStr string) ([]string, error) {
	if path == "" {
		return nil, fmt.Errorf("yt-dlp غير متوفر على الجهاز")
	}
	cctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	out, err := exec.CommandContext(cctx, path, "-F", "--no-warnings", "--skip-download", urlStr).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %s", strings.TrimSpace(string(out)))
	}
	return parseFormatIDs(string(out)), nil
}

// parseFormatIDs يلتقط معرفات الصيغ من جدول yt-dlp.
func parseFormatIDs(out string) []string {
	var ids []string
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 1 && isNum(f[0]) {
			ids = append(ids, f[0])
		}
	}
	return ids
}

func isNum(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
