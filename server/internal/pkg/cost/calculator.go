package cost

// Calc returns yuan cost for the given usage under the given price entry.
// Unknown units return 0 (defensive — surface as a metric/log warning elsewhere).
// Calc is pure: no IO, no global state, no logging.
func Calc(entry PriceEntry, u Usage) float64 {
	switch entry.Unit {
	case "yuan_per_1m_tokens":
		return (float64(u.TokensIn)*entry.InputPrice + float64(u.TokensOut)*entry.OutputPrice) / 1_000_000.0
	case "yuan_per_1k_chars":
		return float64(u.Chars) * entry.CharsPrice / 1000.0
	case "yuan_per_audio_second":
		return u.AudioSeconds * entry.CharsPrice // CharsPrice 字段复用，避免再加字段
	default:
		return 0
	}
}
