// licensing.go — ترخيص محلي فاعل: يرمز الملكية بلا أي اتصال خارجي.
// BaranFlow Pro — مدى الحياة (نشط مسبقًا للمالك Baran).
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const lifeSalt = "baranflow-forever-baran-2026"

// LifetimeKey يولّد مفتاحًا محليًا حاسمًا من بذرة (لأغراض التحقق الذاتي).
func LifetimeKey(seed string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(seed) + "|" + lifeSalt))
	hexs := hex.EncodeToString(sum[:8]) // 16 سداسيًا
	return "BF-LIFE-" + strings.ToUpper(hexs[:4]) + "-" + strings.ToUpper(hexs[4:8]) + "-" + strings.ToUpper(hexs[8:])
}

// ValidateLifetimeKey يتحقق محليًا 100% (بلا خوادم).
func ValidateLifetimeKey(key string) bool {
	return key == LifetimeKey("owner:baran")
}
