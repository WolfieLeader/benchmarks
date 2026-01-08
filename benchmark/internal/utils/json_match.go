package utils

func JsonMatch(want, got any) bool {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return false
		}
		for k, wv := range w {
			gv, exists := g[k]
			if !exists {
				return false
			}
			if !JsonMatch(wv, gv) {
				return false
			}
		}
		return true

	case []any:
		g, ok := got.([]any)
		if !ok {
			return false
		}
		if len(w) != len(g) {
			return false
		}
		for i := range w {
			if !JsonMatch(w[i], g[i]) {
				return false
			}
		}
		return true

	default:
		return want == got
	}
}
