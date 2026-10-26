// downloader_test.go — تحقق حقيقي: إعادة إنتاج الملف بالضبط (تقسيم متوازي + إعادة محاولة + تدفق واحد).
package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// payloadServer يقدّم المحتوى عبر ServeContent (يدعم Range و HEAD بدقة قياسية).
func payloadServer(t *testing.T, payload []byte, name string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(payload))
	}))
}

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func shaOfFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	return hashOf(b)
}

// TestParallelDownloadReassembly: 3MB عشوائي، 7 أجزاء متوازية → مطابقة SHA-256 بالبايت.
func TestParallelDownloadReassembly(t *testing.T) {
	payload := make([]byte, 3<<20) // 3 MB
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}
	srv := payloadServer(t, payload, "payload.bin")
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "out.bin")
	var lastDone, lastTotal int64
	n, err := ParallelDownload(context.Background(), srv.Client(), srv.URL+"/payload.bin", out, 7,
		func(d, tt int64) { lastDone, lastTotal = d, tt })
	if err != nil {
		t.Fatalf("ParallelDownload: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("size: got %d want %d", n, len(payload))
	}
	if got := shaOfFile(t, out); got != hashOf(payload) {
		t.Fatal("إعادة إنتاج الملف تخالف SHA-256 — الأجزاء لم تُجمَّع كما يجب")
	}
	if lastTotal != int64(len(payload)) || lastDone != lastTotal {
		t.Fatalf("progress final: done=%d total=%d", lastDone, lastTotal)
	}
}

// TestParallelDownloadSingleStreamWhenNoRange: خادم يتجاهل Range → تدفق واحد يعيد الملف سليمًا.
func TestParallelDownloadSingleStreamWhenNoRange(t *testing.T) {
	payload := []byte("baranflow-no-range-stream-" + strings.Repeat("x", 500))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// يتجاهل Range عمدًا: دائمًا 200 بكل المحتوى
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	out := filepath.Join(t.TempDir(), "single.bin")
	n, err := ParallelDownload(context.Background(), srv.Client(), srv.URL+"/raw.bin", out, 4, nil)
	if err != nil {
		t.Fatalf("single stream path: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("size: got %d want %d", n, len(payload))
	}
	if got := shaOfFile(t, out); got != hashOf(payload) {
		t.Fatal("محتوى التدفق الواحد مخالف")
	}
}

// TestRetryRecoversBrokenSegment: أول محاولة تُقطع بعد 40 بايت → إعادة المحاولة تكمل من الإزاحة 40.
func TestRetryRecoversBrokenSegment(t *testing.T) {
	payload := make([]byte, 100)
	for i := range payload {
		payload[i] = byte(i)
	}
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// فحص HEAD: ترويسات مقننة فقط
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Content-Range", "bytes 0-99/100")
			w.WriteHeader(http.StatusOK)
			return
		}
		switch r.Header.Get("Range") {
		case "bytes=0-0": // فحص GET بجزء واحد
			w.Header().Set("Content-Range", "bytes 0-0/100")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(payload[:1])
		case "bytes=0-99":
			if hits.Add(1) == 1 {
				// المحاولة الأولى: 40 بايت ثم قطع الاتصال فجأة (خطأ جسم استجابة عند العميل)
				w.Header().Set("Content-Type", "application/octet-stream")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write(payload[:40])
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
				panic(http.ErrAbortHandler)
			}
			http.ServeContent(w, r, "payload.bin", time.Time{}, bytes.NewReader(payload))
		default: // المحاولة الثانية تبدأ من الإزاحة التي توقف عندها المحرك
			http.ServeContent(w, r, "payload.bin", time.Time{}, bytes.NewReader(payload))
		}
	}))
	defer srv.Close()
	defer srv.CloseClientConnections()

	out := filepath.Join(t.TempDir(), "retry.bin")
	n, err := ParallelDownload(context.Background(), srv.Client(), srv.URL+"/p.bin", out, 1, nil)
	if err != nil {
		t.Fatalf("إعادة المحاولة لم تكمل الجزء: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("size: got %d want %d", n, len(payload))
	}
	if got := shaOfFile(t, out); got != hashOf(payload) {
		t.Fatal("بعد الاستكمال خالف المحتوى الأصلي")
	}
}

// TestProbeFilenameAndSize: HEAD يُشبع الحجم والاسم من Content-Disposition.
func TestProbeFilenameAndSize(t *testing.T) {
	payload := []byte("x")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="تقرير-2026.pdf"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	p, err := Probe(context.Background(), srv.Client(), srv.URL+"/anything")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if p.Filename != "تقرير-2026.pdf" {
		t.Fatalf("filename: got %q", p.Filename)
	}
	if p.Size != int64(len(payload)) || p.AcceptRanges {
		t.Fatalf("probe extras: %+v", p)
	}
}

// TestProbeRangesFromURLPath: بلا Content-Disposition → الاسم من المسار.
func TestProbeFilenameFromURL(t *testing.T) {
	srv := payloadServer(t, []byte{1, 2, 3}, "fallback.bin")
	defer srv.Close()
	p, err := Probe(context.Background(), srv.Client(), srv.URL+"/dir/arch/fallback.bin?id=9")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if p.Filename != "fallback.bin" {
		t.Fatalf("filename from URL: got %q", p.Filename)
	}
}

// TestProbeRejectsHTTPError: خادم 404 → خطأ واضح.
func TestProbeRejectsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	if _, err := Probe(context.Background(), srv.Client(), srv.URL+"/ghost"); err == nil {
		t.Fatal("404 قبلت كنجاح")
	}
}

func TestClampSegs(t *testing.T) {
	if got := clampSegs(0, 1<<20); got != 1 {
		t.Fatalf("min clamp: %d", got)
	}
	if got := clampSegs(99, 1<<20); got != 16 {
		t.Fatalf("max clamp: %d", got)
	}
	if got := clampSegs(8, 3); got != 3 {
		t.Fatalf("tiny file clamp: %d", got)
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"../evil":   ".._evil",
		"a/b/c.txt": "a_b_c.txt",
		"con:name":  "con_name",
		"":          "download.bin",
		".":         "download.bin",
		"..":        "download.bin",
		`bad\name`:  "bad_name",
		"ملف شائك":  "ملف شائك",
	}
	for in, want := range cases {
		if got := sanitizeName(in); got != want {
			t.Errorf("sanitizeName(%q) = %q want %q", in, got, want)
		}
	}
}

// TestParallelDownloadContextCancel: الإلغاء يعيد خطأ ولا يعلق (بلا انسداد).
func TestParallelDownloadContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < 200; i++ {
			fmt.Fprint(w, strings.Repeat("Z", 4096))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(30 * time.Millisecond):
			}
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out := filepath.Join(t.TempDir(), "cancelled.bin")
	if _, err := ParallelDownload(ctx, srv.Client(), srv.URL+"/slow", out, 2, nil); err == nil {
		t.Fatal("إلغاء السياق لم يوقف التحميل")
	}
	_ = io.Discard
}
