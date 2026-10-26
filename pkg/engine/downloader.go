// downloader.go — محرك تحميل متعدد الأجزاء (HTTP Range) مع إعادة محاولة لكل جزء.
// آمن خيوطيًا: كل جزء يكتب WriteAt على إزاحة خاصة به في ملف واحد.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultUserAgent = "BaranFlow/0.1 (+local-sovereign; owner=Baran)"

// ProbeResult نتيجة الفحص الأولي للمورد.
type ProbeResult struct {
	Size         int64
	AcceptRanges bool
	ContentType  string
	Filename     string
}

func doRequest(ctx context.Context, client *http.Client, method, urlStr string, setRange bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	if setRange {
		req.Header.Set("Range", "bytes=0-0")
	}
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req)
}

// Probe يعرف الحجم وقابلية النطاق واسم الملف (HEAD ثم GET بجزء).
func Probe(ctx context.Context, client *http.Client, urlStr string) (*ProbeResult, error) {
	if resp, err := doRequest(ctx, client, http.MethodHead, urlStr, false); err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return parseProbe(resp, urlStr)
		}
	}
	resp, err := doRequest(ctx, client, http.MethodGet, urlStr, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	return parseProbe(resp, urlStr)
}

func parseProbe(resp *http.Response, urlStr string) (*ProbeResult, error) {
	p := &ProbeResult{ContentType: resp.Header.Get("Content-Type")}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > 0 {
			p.Size = n
		}
	}
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		parts := strings.SplitN(cr, "/", 2)
		if len(parts) == 2 {
			if n, err := strconv.ParseInt(parts[1], 10, 64); err == nil && n > 0 {
				p.Size = n
			}
		}
	}
	if ar := strings.ToLower(resp.Header.Get("Accept-Ranges")); ar == "bytes" || resp.StatusCode == http.StatusPartialContent {
		p.AcceptRanges = true
	}
	p.Filename = cleanFilename(resp.Header.Get("Content-Disposition"), urlStr)
	return p, nil
}

func cleanFilename(disposition, urlStr string) string {
	if disposition != "" {
		if i := strings.Index(strings.ToLower(disposition), "filename="); i >= 0 {
			rest := strings.TrimSpace(disposition[i+9:])
			rest = strings.Trim(rest, `"'`)
			if j := strings.Index(rest, ";"); j >= 0 {
				rest = strings.TrimSpace(strings.Trim(rest[:j], `"'`))
			}
			if rest != "" && !strings.ContainsAny(rest, `/\`) {
				return sanitizeName(rest)
			}
		}
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return "download.bin"
	}
	name := path.Base(u.Path)
	return sanitizeName(name)
}

func sanitizeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(`/:*?"<>|\`, r) {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." || out == "/" {
		return "download.bin"
	}
	return out
}

type ProgressFn func(done, total int64)

// ParallelDownload يحمّل الملف بأجزاء متوازية أو تدفقًا واحدًا إن لم يدعم الخادم النطاق.
func ParallelDownload(ctx context.Context, client *http.Client, urlStr, out string, segs int, pg ProgressFn) (int64, error) {
	probe, err := Probe(ctx, client, urlStr)
	if err != nil {
		return 0, fmt.Errorf("probe: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	total := probe.Size
	if pg != nil && total > 0 {
		pg(0, total)
	}

	if !probe.AcceptRanges || total <= 0 {
		return copyWhole(ctx, client, urlStr, f, total, pg)
	}

	segs = clampSegs(segs, total)
	step := (total + int64(segs) - 1) / int64(segs)

	var agg atomic.Int64
	errc := make(chan error, segs)
	var wg sync.WaitGroup

	for i := 0; i < segs; i++ {
		start := int64(i) * step
		if start >= total {
			break
		}
		end := start + step - 1
		if end > total-1 {
			end = total - 1
		}
		wg.Add(1)
		go func(start, end int64) {
			defer wg.Done()
			var segDone int64
			var lastErr error
			for attempt := 0; attempt < 3; attempt++ {
				if ctx.Err() != nil {
					lastErr = ctx.Err()
					break
				}
				err := fetchRange(ctx, client, urlStr, start+segDone, end, f, &segDone, &agg)
				if err == nil {
					return
				}
				if ctx.Err() != nil {
					lastErr = ctx.Err()
					break
				}
				lastErr = err
				select {
				case <-time.After(time.Duration(attempt+1) * 700 * time.Millisecond):
				case <-ctx.Done():
					lastErr = ctx.Err()
				}
			}
			errc <- fmt.Errorf("segment %d: %w", start, lastErr)
		}(start, end)
	}

	wg.Wait()
	close(errc)
	var firstErr error
	for e := range errc {
		if firstErr == nil {
			firstErr = e
		}
	}
	if firstErr != nil {
		return agg.Load(), firstErr
	}
	if agg.Load() != total {
		return agg.Load(), fmt.Errorf("incomplete: %d/%d", agg.Load(), total)
	}
	if pg != nil {
		pg(total, total)
	}
	return total, nil
}

func clampSegs(segs int, total int64) int {
	if segs < 1 {
		segs = 1
	}
	if segs > 16 {
		segs = 16
	}
	if int64(segs) > total {
		segs = int(total)
	}
	if segs < 1 {
		segs = 1
	}
	return segs
}

// fetchRange يعيد المحاولة من إزاحة الحالة الحالية (استكمال منطقي داخل الجزء).
func fetchRange(ctx context.Context, client *http.Client, urlStr string, start, end int64, f *os.File, segDone *int64, agg *atomic.Int64) error {
	if start > end {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && start != 0 {
		return errors.New("server ignored Range header")
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	buf := make([]byte, 64*1024)
	pos := start
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			w, werr := f.WriteAt(buf[:n], pos)
			if werr != nil {
				return werr
			}
			pos += int64(w)
			atomic.AddInt64(segDone, int64(w))
			agg.Add(int64(w))
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// copyWhole نسخة تدفق واحد لخدمات لا تدعم النطاق.
func copyWhole(ctx context.Context, client *http.Client, urlStr string, f *os.File, knownTotal int64, pg ProgressFn) (int64, error) {
	resp, err := doRequest(ctx, client, http.MethodGet, urlStr, false)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusPartialContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return 0, errors.New("unexpected 206")
	}
	buf := make([]byte, 64*1024)
	var done int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			_, werr := f.WriteAt(buf[:n], done)
			if werr != nil {
				return done, werr
			}
			done += int64(n)
			if pg != nil {
				pg(done, knownTotal)
			}
		}
		if rerr == io.EOF {
			if pg != nil {
				pg(done, done)
			}
			return done, nil
		}
		if rerr != nil {
			return done, rerr
		}
	}
}
