# البنية البرمجية — BaranFlow v0.1

خريطة المواصفة التي طلبتها إلى ما نُفِّذ فعليًا (نمط Layered/MVC مضغوط):

| الطبقة (كما في المواصفة) | التنفيذ هنا |
|---|---|
| واجهة المستخدم (Presentation) | `webui/` — صفحة واحدة عربية RTL مظلمة، مدمجة في الثنائي عبر `embed` |
| المتحكم (Controller) | `cmd/baranflowd/main.go` — REST API: analyze/tasks/stats/logs/tools/license |
| المنطق الأساسي (Core Engines) | `pkg/engine/` — محرك متعدد المناطق (downloader.go)، محلل (analyzer.go)، جسر خارجي (external.go)، مدير طوابير (queue.go) |
| الأدوات الخدمية | `pkg/util/` — تقسيم/دمج/ضغط/استخراج صوت + ترخيص |
| التخزين والبيانات | `pkg/storage/` — JSON منظم بذري atomic (بدل SQLite في v0.1 — حسب مواصفتك «أو ملفات حالات») |

## التزامن
- كل مهمة = goroutine، كل جزء (Segment) = goroutine `fetchRange` بـ `WriteAt` على إزاحات مختلفة (آمن خيوطيًا).
- مزامنة عبر `sync.Mutex` واحد للمدير؛ عدادات `atomic` للتقدم.

## حدود صادقة (لن ندّعي غيرها)
- استكمال التحميل الجزئي (Resume) — v0.2 (يحتاج خريطة أجزاء على القرص).
- التورنت الكامل — عبر محرك خارجي (aria2c/transmission) في v0.2، لا انهناد بروتوكول BitTorrent من الصفر في v0.1.
- حظر الإعلانات والمتصفح الآمن — v0.2.
