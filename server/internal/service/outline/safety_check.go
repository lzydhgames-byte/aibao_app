package outline

import (
	"context"
	"errors"
	"strings"

	"github.com/aibao/server/internal/service/safety"
)

// OutlineSafetyCheck error categories (spec §5.3). Categories align with
// safety taxonomy used by PostCheck so unified metric labels stay consistent.
var (
	ErrSafetyRedline            = errors.New("outline safety: redline word hit")
	ErrSafetyChildFears         = errors.New("outline safety: child fears hit")
	ErrSafetyProtagonistMissing = errors.New("outline safety: child not the protagonist")
	ErrSafetyIPMisuse           = errors.New("outline safety: IP misuse (IP as protagonist)")
)

// SafetyCheckInput is the input for Check.
// All slice fields are optional (nil/empty = skip that sub-check).
type SafetyCheckInput struct {
	Outline       RawOutline
	ChildNickname string
	ChildFears    []string // personalized fears from child profile / [[bootstrap-fears]]
	IPBlacklist   []string
	IPWhitelist   []string // 可作"陪伴角色"出现，但不能是主角（标题命中视为抢主角）
}

// SafetyCheckResult.Category aligns with safety category taxonomy
// for unified reporting / metric labels (outline_safety_repair_total{category}).
type SafetyCheckResult struct {
	OK       bool
	Reason   error
	Category string // "redline" / "fears_personalized" / "protagonist_missing" / "ip_misuse"
}

// Check inspects title + synopsis + educational_value for safety violations.
// Returns first violation; caller may attempt 1 repair retry (spec §5.3).
//
// Order of checks (fail-fast on first hit):
//  1. Redline word scan on combined title+synopsis+educational_value
//  2. Personalized child fears scan on combined
//  3. Protagonist check: synopsis MUST contain child nickname (if provided)
//  4. IP blacklist hit in synopsis -> reject
//  5. IP whitelist hit in TITLE only -> reject
//     (whitelist OK in synopsis = "companion"; title = "protagonist seat")
func Check(ctx context.Context, matcher safety.Matcher, in SafetyCheckInput) SafetyCheckResult {
	combined := in.Outline.Title + "\n" + in.Outline.Synopsis + "\n" + in.Outline.EducationalValue

	// 1. Redline scan (reuse safety.Matcher).
	if matcher != nil {
		if hit, ok := matcher.FindFirst(combined); ok && hit != "" {
			return SafetyCheckResult{Reason: ErrSafetyRedline, Category: "redline"}
		}
	}

	// 2. Personalized child fears.
	for _, fear := range in.ChildFears {
		if fear == "" {
			continue
		}
		if strings.Contains(combined, fear) {
			return SafetyCheckResult{Reason: ErrSafetyChildFears, Category: "fears_personalized"}
		}
	}

	// 3. Protagonist check: synopsis must mention child nickname.
	if in.ChildNickname != "" && !strings.Contains(in.Outline.Synopsis, in.ChildNickname) {
		return SafetyCheckResult{Reason: ErrSafetyProtagonistMissing, Category: "protagonist_missing"}
	}

	// 4. IP blacklist in synopsis -> reject.
	for _, ip := range in.IPBlacklist {
		if ip == "" {
			continue
		}
		if strings.Contains(in.Outline.Synopsis, ip) {
			return SafetyCheckResult{Reason: ErrSafetyIPMisuse, Category: "ip_misuse"}
		}
	}

	// 5. IP whitelist in TITLE (synopsis OK, just title = "抢主角位").
	for _, ip := range in.IPWhitelist {
		if ip == "" {
			continue
		}
		if strings.Contains(in.Outline.Title, ip) {
			return SafetyCheckResult{Reason: ErrSafetyIPMisuse, Category: "ip_misuse"}
		}
	}

	return SafetyCheckResult{OK: true}
}
