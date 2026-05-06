package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskPhone(t *testing.T) {
	assert.Equal(t, "138****8000", MaskPhone("13800138000"))
	assert.Equal(t, "***", MaskPhone("123"))
	assert.Equal(t, "", MaskPhone(""))
	assert.Equal(t, "138****8000", MaskPhone("+8613800138000"), "should normalize country code prefix")
}

func TestRedactPromptText(t *testing.T) {
	in := "想听一个奥特曼打怪兽的睡前故事"
	out := RedactPromptText(in)
	assert.NotContains(t, out, "奥特曼")
	assert.Contains(t, out, "len=")
}

func TestRedactPromptText_Empty(t *testing.T) {
	assert.Equal(t, "len=0", RedactPromptText(""))
}
