package model

import (
	"strings"
	"time"
)

// Mood constants drive BGM selection and ffmpeg input lookup.
// 对应 LLM 输出 cue 的中文 → 英文目录/key。
const (
	MoodWarm      = "warm"      // 温馨治愈
	MoodAdventure = "adventure" // 冒险探索
	MoodFunny     = "funny"     // 搞笑欢乐
	MoodMagic     = "magic"     // 神奇魔法
	MoodCurious   = "curious"   // 科普认知
)

// BGMAsset is one row in bgm_assets.
type BGMAsset struct {
	ID          int64     `gorm:"column:id;primaryKey" json:"id"`
	Mood        string    `gorm:"column:mood" json:"mood"`
	Filename    string    `gorm:"column:filename" json:"filename"`
	ObjectKey   string    `gorm:"column:object_key" json:"object_key"`
	DurationSec int       `gorm:"column:duration_sec" json:"duration_sec"`
	License     string    `gorm:"column:license" json:"license"`
	Active      bool      `gorm:"column:active" json:"active"`
	CreatedAt   time.Time `gorm:"column:created_at" json:"created_at"`
}

func (BGMAsset) TableName() string { return "bgm_assets" }

// MoodFromStyle maps story.Style (Chinese) to mood key.
// Unknown style falls back to MoodWarm.
func MoodFromStyle(style string) string {
	switch strings.TrimSpace(style) {
	case "温馨治愈", "温馨", "治愈":
		return MoodWarm
	case "冒险探索", "冒险", "探险":
		return MoodAdventure
	case "搞笑欢乐", "搞笑", "欢乐":
		return MoodFunny
	case "神奇魔法", "神奇", "魔法":
		return MoodMagic
	case "科普认知", "科普", "认知":
		return MoodCurious
	default:
		return MoodWarm
	}
}

// MoodFromCueLabel maps the Chinese cue label emitted by LLM to mood key.
// Unknown label falls back to MoodWarm.
func MoodFromCueLabel(label string) string {
	switch strings.TrimSpace(label) {
	case "温馨", "温馨治愈", "治愈":
		return MoodWarm
	case "冒险", "冒险探索", "探险":
		return MoodAdventure
	case "搞笑", "搞笑欢乐", "欢乐":
		return MoodFunny
	case "魔法", "神奇", "神奇魔法":
		return MoodMagic
	case "科普", "好奇", "认知", "科普认知":
		return MoodCurious
	default:
		return MoodWarm
	}
}
