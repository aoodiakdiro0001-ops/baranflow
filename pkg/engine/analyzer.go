// analyzer.go — تحليل الرابط: النوع/المنصة/الاسم/الحجم (يعمل دون شبكة للمنصات المعروفة).
package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// AnalyzedFile نتيجة التحليل.
type AnalyzedFile struct {
	URL          string   `json:"url"`
	Filename     string   `json:"filename"`
	ContentType  string   `json:"content_type"`
	SizeBytes    int64    `json:"size_bytes"`
	AcceptRanges bool     `json:"accept_ranges"`
	Kind         string   `json:"kind"`     // file|video|torrent|page
	Platform     string   `json:"platform"` // YouTube/TikTok/...
	Options      []string `json:"options"`
	Notes        string   `json:"notes"`
}

// Platforms خرائط الشبكات المدعومة عبر الجسر الخارجي (yt-dlp).
var platforms = map[string]string{
	"youtube.com":     "YouTube",
	"youtu.be":        "YouTube",
	"tiktok.com":      "TikTok",
	"instagram.com":   "Instagram",
	"facebook.com":    "Facebook",
	"fb.watch":        "Facebook",
	"vimeo.com":       "Vimeo",
	"dailymotion.com": "Dailymotion",
	"soundcloud.com":  "SoundCloud",
	"x.com":           "X (Twitter)",
	"twitter.com":     "X (Twitter)",
}

func DetectPlatform(host string) string {
	h := strings.ToLower(strings.TrimPrefix(host, "www."))
	for k, v := range platforms {
		if h == k || strings.HasSuffix(h, "."+k) {
			return v
		}
	}
	return ""
}

// Analyze يحلل الرابط: منصة وسائط بدون شبكة، وإلا فحص رأس مباشر.
func Analyze(ctx context.Context, client *http.Client, urlStr string, tools Tools) (*AnalyzedFile, error) {
	raw := strings.TrimSpace(urlStr)
	if raw == "" {
		return nil, fmt.Errorf("رابط فارغ")
	}

	// مغناطيس/تورنت — قبل أي تحليل شبكي
	if strings.HasPrefix(raw, "magnet:?") {
		return &AnalyzedFile{URL: raw, Kind: "torrent", Platform: "BitTorrent (Magnet)",
			Notes: "تورنت عبر جسر aria2/transmission (v0.2) — الشيء الصادق لا الوهم"}, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("رابط غير صالح: %v", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return nil, fmt.Errorf("المخططات المدعومة: http/https فقط")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("لا يوجد خادم في الرابط")
	}

	a := &AnalyzedFile{URL: raw, Options: []string{}}
	if strings.EqualFold(u.Path, "") && strings.HasSuffix(strings.ToLower(raw), ".torrent") {
		a.Kind = "torrent"
		a.Platform = "BitTorrent"
	}

	if pf := DetectPlatform(u.Host); pf != "" {
		a.Kind = "video"
		a.Platform = pf
		a.Options = []string{"best", "1080p", "720p", "480p", "audio-only"}
		if tools.YtDlp == "" {
			a.Notes = "yt-dlp غير مثبت — الأداة تكشف ولا تكذب؛ ثبّته لتشغيل الفيديو"
		} else {
			a.Notes = "yt-dlp متوفر — سيُستخدم حسب خيارات الجودة (v0.2 الجسر الكامل)"
		}
		return a, nil
	}

	probe, err := Probe(ctx, client, raw)
	if err != nil {
		return nil, err
	}
	a.Filename = probe.Filename
	a.ContentType = probe.ContentType
	a.SizeBytes = probe.Size
	a.AcceptRanges = probe.AcceptRanges
	ct := strings.ToLower(probe.ContentType)
	switch {
	case strings.HasPrefix(ct, "video/") || strings.HasPrefix(ct, "audio/"):
		a.Kind = "video"
	case strings.Contains(ct, "x-bittorrent"):
		a.Kind = "torrent"
	case strings.HasPrefix(ct, "text/html"):
		a.Kind = "page"
		a.Notes = "صفحة ويب — يفضل رابط المورد المباشر أو رابط منصة الوسائط"
	default:
		a.Kind = "file"
	}
	if a.Filename == "" {
		a.Filename = "download.bin"
	}
	if a.Kind == "" {
		a.Kind = "file"
	}
	return a, nil
}
