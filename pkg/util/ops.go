// ops.go — أدوات سريعة: تقسيم/دمج/ضغط (داخلية، بلا اعتماديات) + استخراج صوت (ffmpeg إن توفر).
package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SplitFile يقسم ملفًا إلى parts أجزاء متساوية تقريبًا: outPrefix.aa .ab ...
func SplitFile(src string, outPrefix string, parts int) error {
	if parts <= 1 || parts > 512 {
		return fmt.Errorf("عدد الأجزاء يجب أن يكون بين 2 و 512")
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	total := st.Size()
	chunk := (total + int64(parts) - 1) / int64(parts)
	if chunk <= 0 {
		return fmt.Errorf("الملف صغير جدًا للقسمة")
	}
	// اسم لاحقة مزدوجة
	letters := "abcdefghijklmnopqrstuvwxyz"
	base := filepath.Base(outPrefix)
	for i := 0; i < parts && int64(i)*chunk < total; i++ {
		out := fmt.Sprintf("%s.%c%c", base, letters[(i/26)%26], letters[i%26])
		w, err := os.Create(filepath.Join(filepath.Dir(src), out))
		if err != nil {
			return err
		}
		if _, err := io.CopyN(w, f, chunk); err != nil {
			if err == io.EOF {
				_ = w.Close()
				break
			}
			_ = w.Close()
			return err
		}
		_ = w.Close()
	}
	return nil
}

// JoinFiles يدمج أجزاء مسلسلة (بالترتيب الممرر) في ملف واحد.
func JoinFiles(parts []string, dest string) error {
	if len(parts) < 1 {
		return fmt.Errorf("لا أجزاء للدمج")
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	for _, p := range parts {
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			return err
		}
		_ = in.Close()
	}
	return nil
}

// CompressZip يضغط ملفًا أو مجلدًا إلى ZIP داخلية (بلا أدوات خارجية).
func CompressZip(src, dest string) error {
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	defer zw.Close()
	base := filepath.Dir(src)
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, path)
		if rel == "." {
			return nil
		}
		h, _ := zip.FileInfoHeader(info)
		h.Name = rel
		if info.IsDir() {
			h.Name += "/"
			_, err := zw.CreateHeader(h)
			return err
		}
		fw, _ := zw.CreateHeader(h)
		in, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		defer in.Close()
		_, err = io.Copy(fw, in)
		return err
	})
}

// ExtractAudio يستخرج مسارًا صوتيًا عبر ffmpeg — يعمل فقط إن توفر ffmpeg.
func ExtractAudio(ffmpegPath, src, dest string) error {
	if ffmpegPath == "" {
		return fmt.Errorf("ffmpeg غير متوفر — ثبّته لتشغيل استخراج الصوت (الأداة خارجية فقط)")
	}
	cmd := exec.Command(ffmpegPath, "-y", "-i", src, "-vn", dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
