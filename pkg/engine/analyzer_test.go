// analyzer_test.go — تحقق التحليل: منصات بلا شبكة، مغناطيس، ملفات HTTP، رفض المخططات الغريبة.
package engine

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// panicTransport ينفجر عند أي طلب شبكي — يثبت عدم حدوث اتصال خفي.
type panicTransport struct{}

func (panicTransport) RoundTrip(*http.Request) (*http.Response, error) {
	panic("اتصال شبكي غير متوقع داخل التحليل")
}

func TestDetectPlatform(t *testing.T) {
	cases := map[string]string{
		"youtube.com":       "YouTube",
		"www.youtube.com":   "YouTube",
		"youtu.be":          "YouTube",
		"m.tiktok.com":      "TikTok",
		"tiktok.com":        "TikTok",
		"x.com":             "X (Twitter)",
		"twitter.com":       "X (Twitter)",
		"www.instagram.com": "Instagram",
		"fb.watch":          "Facebook",
		"vimeo.com":         "Vimeo",
		"soundcloud.com":    "SoundCloud",
		"example.com":       "",
		"evil-youtube.com":  "", // احتياج لاحقة نقطية كاملة
	}
	for host, want := range cases {
		if got := DetectPlatform(host); got != want {
			t.Errorf("DetectPlatform(%q) = %q want %q", host, got, want)
		}
	}
}

func TestAnalyzePlatformWithoutNetwork(t *testing.T) {
	a, err := Analyze(context.Background(), &http.Client{Transport: panicTransport{}},
		"https://www.youtube.com/watch?v=abc123", Tools{})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if a.Kind != "video" || a.Platform != "YouTube" {
		t.Fatalf("kind/platform: %s/%s", a.Kind, a.Platform)
	}
	if len(a.Options) == 0 {
		t.Fatal("خيارات جودة فارغة")
	}
	if a.Notes == "" || !strings.Contains(a.Notes, "yt-dlp") {
		t.Fatalf("ملاحظة الصدق مفقودة: %q", a.Notes)
	}
}

func TestAnalyzeMagnetWithoutNetwork(t *testing.T) {
	a, err := Analyze(context.Background(), &http.Client{Transport: panicTransport{}},
		"magnet:?xt=urn:btih:ABCDEF&dn=movie", Tools{})
	if err != nil {
		t.Fatalf("Analyze magnet: %v", err)
	}
	if a.Kind != "torrent" || a.Platform != "BitTorrent (Magnet)" {
		t.Fatalf("magnet: kind=%s platform=%s", a.Kind, a.Platform)
	}
}

func TestAnalyzeRejectsBadInput(t *testing.T) {
	bad := []string{
		"",
		"   ",
		"ftp://example.com/x",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"http:///no-host",
	}
	for _, u := range bad {
		if _, err := Analyze(context.Background(), http.DefaultClient, u, Tools{}); err == nil {
			t.Errorf("قبلت رابطًا خاطئًا: %q", u)
		}
	}
}

// TestAnalyzeHTTPFileMultiContent:ServeContent يغطي أنواع ملف/فيديو/تورنت/صفحة.
func TestAnalyzeHTTPContentTypes(t *testing.T) {
	cases := []struct {
		name string
		kind string
	}{
		{"archive.bin", "file"},
		{"movie.mp4", "video"},
		{"song.mp3", "video"}, // الصوت يصنَّف وسائط
		{"island.torrent", "torrent"},
		{"index.html", "page"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("baran"), 2048)
			srv := payloadServer(t, payload, c.name)
			defer srv.Close()

			a, err := Analyze(context.Background(), srv.Client(),
				srv.URL+"/dir/sub/"+c.name+"?tok=1", Tools{})
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if a.Kind != c.kind {
				t.Errorf("kind = %q want %q (content_type=%s)", a.Kind, c.kind, a.ContentType)
			}
			if a.SizeBytes != int64(len(payload)) {
				t.Errorf("size = %d want %d", a.SizeBytes, len(payload))
			}
			if a.Filename != c.name {
				t.Errorf("filename = %q want %q", a.Filename, c.name)
			}
			if !a.AcceptRanges {
				t.Errorf("ServeContent يجب أن يعلن دعم النطاقات")
			}
		})
	}
}

// TestAnalyzeUnreachable: خادم ميت → خطأ شفاف لا وهم.
func TestAnalyzeUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL + "/gone"
	srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := Analyze(ctx, http.DefaultClient, url, Tools{}); err == nil {
		t.Fatal("قبلت خادمًا ميتًا")
	}
}

// TestAnalyzePageKindNotes: صفحة HTML تحمل ملاحظة إرشادية.
func TestAnalyzePageKindNotes(t *testing.T) {
	srv := payloadServer(t, []byte("<html></html>"), "index.html")
	defer srv.Close()
	a, err := Analyze(context.Background(), srv.Client(), srv.URL+"/", Tools{})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if a.Kind != "page" || a.Notes == "" {
		t.Fatalf("kind/notes: %s/%q", a.Kind, a.Notes)
	}
}
