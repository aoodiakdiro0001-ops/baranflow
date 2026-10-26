# BaranFlow — حزمة النشر (PUBLISH_KIT) — Baran © 2026

> كل النصوص جاهزة للنسخ/اللصق. ارفعها إلى قنواتك (X · Reddit · Hacker News · LinkedIn) نسخةً نسخة.
> الأمانة أولًا: هذه النصوص تذكر حدود v0.1 كما هي — بلا مبالغة.

---

## 1) X (تويتر — منشور واحد قصير)

**BaranFlow ⚡** مدير تنزيلات عربي مظلم (RTL) يعمل محليًا على جهازك:
- محرك تحميل متعدد الأجزاء (حتى 16) مع إعادة محاولة لكل جزء
- تحليل روابط ذكي + جسر منصات فيديو عبر yt-dlp إن وُجد
- أدوات سريعة: تقسيم/دمج/ضغط — كله داخل الجهاز، بلا سحابة

Go قياسي + واجهة مدمجة في الثنائي (نسخة واحدة تشغّل كل شيء).
[github.com/aoodiakdiro0001-ops/baranflow]
الحقوق: © 2026 Baran

---

## 2) Reddit — r/golang (عنوان + نص)

**العنوان:**
BaranFlow: a single-binary Go download manager with an embedded dark RTL UI — multi-segment HTTP engine, on-device tools, zero cloud

**نص المنشور:**
I built BaranFlow — a local-first download/media manager daemon in Go (single ~10MB static binary that embeds the web UI; no npm, no build step for the frontend).

What it does in v0.1:
- Multi-segment HTTP downloads (up to 16 parallel ranges) with per-segment retry that resumes from the byte offset where the attempt broke — verified by a test that deliberately aborts a response mid-body.
- Link analysis: content-type / size / range support / filename from Content-Disposition; video platforms are detected and bridged through yt-dlp **only if it exists on the machine** — the UI states missing tools honestly instead of faking options.
- Queue manager: parallel task limit, per-task speed (EMA), events ring, JSON task persistence across restarts.
- Built-in quick ops: file split/join, ZIP compression (stdlib), audio extraction via ffmpeg if installed.
- Embedded Arabic dark RTL web UI served from the binary itself.

Run: `go run ./cmd/baranflowd` → http://127.0.0.1:4890

What it does NOT do yet (v0.2): true BitTorrent (needs an external bridge like aria2/transmission — v0.1 analyzes magnets and refuses them honestly), partial resume.

Repo: https://github.com/aoodiakdiro0001-ops/baranflow
Feedback welcome — especially on the retry/range engine.

---

## 3) Hacker News — تنسيق "Show HN"

**العنوان (بدون زخرفة):**
Show HN: BaranFlow – single-binary local download manager (Go) with embedded RTL UI

**النص التعليقي:**
Hi. BaranFlow is a small Go daemon: paste a URL, it HEAD-probes the resource, decides single-stream vs. N parallel ranges, downloads with per-segment retry (resume-from-offset within a segment), and serves an Arabic dark RTL UI — embedded in the binary with go:embed.

Design constraints I kept:
- Local-first: no telemetry, no accounts, no cloud. State is one JSON file.
- Honesty over polish: capability checks tell you when yt-dlp/ffmpeg/aria2c are missing rather than showing dead buttons.
- Segments cap at 16; parallel tasks default to 3.

Known limits (v0.1): no torrent engine (magnet links return an explicit refusal), no partial-file resume.

Code/tests: https://github.com/aoodiakdiro0001-ops/baranflow (pkg/engine has the reassembly + deliberately-broken-connection tests).

---

## 4) LinkedIn (احترافي مختصر)

**BaranFlow v0.1.0** — أصدق مدير تنزيلات محلي: ثنائي Go واحد يضم الواجهة العربية (RTL/Dark)، محرك HTTP متعدد الأجزاء استُعيد في الاختبارات بايتًا بايتًا، طوابير موازية، وأدوات تقسيم/دمج/ضغط. لا سحابة — لا وعد كاذب: أي أداة مفقودة تُعلن بصراحة.
المستودع: github.com/aoodiakdiro0001-ops/baranflow
الحقوق والملكية: Baran © 2026 · baranavisat@gmail.com

---

## 5) بطاقة الدعم (مثبتة في كل منشور)

باران (Baran)
📧 baranavisat@gmail.com
📞 هاتف/واتساب: +90 531 326 37 22 — https://wa.me/905313263722
