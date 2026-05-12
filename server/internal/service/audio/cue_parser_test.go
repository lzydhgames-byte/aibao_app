package audio

import (
	"testing"

	"github.com/aibao/server/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestParse_StripsMarkers(t *testing.T) {
	in := "从前有一只小熊[音效:风声]他走在森林里。[BGM情绪:温馨]终于回家了。"
	r := Parse(in, "温馨治愈")
	assert.Equal(t, "从前有一只小熊他走在森林里。终于回家了。", r.CleanText)
	assert.Len(t, r.Cues, 2)
	assert.Equal(t, CueTypeSFX, r.Cues[0].Type)
	assert.Equal(t, "风声", r.Cues[0].Label)
	assert.Equal(t, CueTypeBGM, r.Cues[1].Type)
	assert.Equal(t, model.MoodWarm, r.BGMMood)
}

func TestParse_NoBGMCue_FallbackStyle(t *testing.T) {
	r := Parse("纯文本无 cue", "冒险探索")
	assert.Equal(t, "纯文本无 cue", r.CleanText)
	assert.Empty(t, r.Cues)
	assert.Equal(t, model.MoodAdventure, r.BGMMood)
}

func TestParse_NoMarkers_NoStyle_DefaultsWarm(t *testing.T) {
	r := Parse("光秃秃", "")
	assert.Equal(t, model.MoodWarm, r.BGMMood)
}

func TestParse_UnknownBGMLabel_FallbackToStyle(t *testing.T) {
	in := "[BGM情绪:暴风]开头"
	r := Parse(in, "冒险探索")
	assert.Equal(t, "开头", r.CleanText)
	assert.Len(t, r.Cues, 1)
	assert.Equal(t, model.MoodAdventure, r.BGMMood)
}

func TestParse_FirstBGMCueWins(t *testing.T) {
	in := "[BGM情绪:温馨]开头[BGM情绪:冒险]中段"
	r := Parse(in, "")
	assert.Equal(t, model.MoodWarm, r.BGMMood)
	assert.Len(t, r.Cues, 2)
}

func TestParse_OffsetsAreIntoCleanText(t *testing.T) {
	in := "ABC[音效:bell]DEF"
	r := Parse(in, "")
	assert.Equal(t, "ABCDEF", r.CleanText)
	assert.Equal(t, 3, r.Cues[0].CharOffset)
}

func TestParse_MultipleSFXAndBGM_OrderPreserved(t *testing.T) {
	in := "A[音效:门]B[BGM情绪:温馨]C[音效:雨]D"
	r := Parse(in, "")
	assert.Equal(t, "ABCD", r.CleanText)
	assert.Len(t, r.Cues, 3)
	assert.Equal(t, CueTypeSFX, r.Cues[0].Type)
	assert.Equal(t, "门", r.Cues[0].Label)
	assert.Equal(t, 1, r.Cues[0].CharOffset)
	assert.Equal(t, CueTypeBGM, r.Cues[1].Type)
	assert.Equal(t, 2, r.Cues[1].CharOffset)
	assert.Equal(t, CueTypeSFX, r.Cues[2].Type)
	assert.Equal(t, "雨", r.Cues[2].Label)
	assert.Equal(t, 3, r.Cues[2].CharOffset)
	assert.Equal(t, model.MoodWarm, r.BGMMood)
}
