// util.go — أدوات مساعدة عامة (زمن، صادقة بلا موجات).
package storage

import "time"

func timeNow() int64 { return time.Now().UTC().Unix() }
