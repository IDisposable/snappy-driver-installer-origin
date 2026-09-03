package settings

import "strings"

// The original C++ engine's sdio.cfg used colon-glued switches with
// underscored names (-drp_dir:value). This rewrite's flag.FlagSet uses
// idiomatic hyphenated "-name=value" syntax instead. The tables below
// translate the old syntax to the new one when reading an existing
// cfg file, so users don't have to hand-edit settings they already
// have on disk. Settings.Save always writes the new syntax.
var legacyPrefixRenames = []struct{ from, to string }{
	{"-drp_dir:", "-drp-dir="},
	{"-index_dir:", "-index-dir="},
	{"-output_dir:", "-output-dir="},
	{"-data_dir:", "-data-dir="},
	{"-log_dir:", "-log-dir="},
	{"-finish_cmd:", "-finish-cmd="},
	{"-finishrb_cmd:", "-finish-reboot-cmd="},
	{"-finish_upd_cmd:", "-finish-update-cmd="},
	{"-filters:", "-filters="},
	{"-ls:", "-ls="},
	{"-getdevicelist:", "-device-list="},
	{"-v:", "-virtual-os-version="},
	{"-extractdir:", "-extractdir="},
}

var legacyExactRenames = map[string]string{
	"-a:32":     "-arch=32",
	"-a:64":     "-arch=64",
	"-index_hr": "-index-hr",
}

// legacyDroppedPrefixes and legacyDroppedExact are switches the
// original engine had that this rewrite doesn't (GUI presentation
// state, torrent tuning not yet ported). They're silently ignored
// when reading an existing cfg file instead of causing a hard parse
// error.
var legacyDroppedPrefixes = []string{
	"-lang:", "-theme:", "-hintdelay:", "-license:",
	"-wndwx:", "-wndwy:", "-wndsc:", "-scale:", "-verbose:",
	"-port:", "-minport:", "-maxport:", "-downlimit:", "-uplimit:", "-connections:",
}

var legacyDroppedExact = map[string]bool{
	"-expertmode":    true,
	"-showconsole":   true,
	"-showdrpnames1": true,
	"-showdrpnames2": true,
	"-oldstyle":      true,
}

// translateLegacyArg rewrites a single token from an existing cfg file
// to this rewrite's flag syntax. It reports false if the token should
// be dropped entirely (an obsolete setting with no equivalent here).
func translateLegacyArg(tok string) (string, bool) {
	if strings.HasPrefix(tok, "/") {
		tok = "-" + tok[1:]
	}

	lower := strings.ToLower(tok)
	if legacyDroppedExact[lower] {
		return "", false
	}
	for _, p := range legacyDroppedPrefixes {
		if hasFoldPrefix(tok, p) {
			return "", false
		}
	}
	if to, ok := legacyExactRenames[lower]; ok {
		return to, true
	}
	for _, r := range legacyPrefixRenames {
		if hasFoldPrefix(tok, r.from) {
			return r.to + tok[len(r.from):], true
		}
	}
	return tok, true
}

func hasFoldPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}
