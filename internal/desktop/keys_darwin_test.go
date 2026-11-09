//go:build darwin

package desktop

import (
	"testing"

	"github.com/thescottbot/punchline/internal/hotkey"
)

func TestEverySupportedKeyIsMappedOrExcluded(t *testing.T) {
	for _, k := range hotkey.Supported() {
		_, mapped := codeByKey[k]
		if mapped == unsupportedOnMac[k] {
			t.Errorf("%s: mapped=%v excluded=%v", k, mapped, unsupportedOnMac[k])
		}
	}
	for k := range codeByKey {
		if !hotkey.IsSupported(k) {
			t.Errorf("%s mapped but not a supported key", k)
		}
	}
	if codeByKey["F13"] != 0x69 || codeByKey["F14"] != 0x6B {
		t.Error("F13/F14 codes")
	}
	seen := map[uint16]hotkey.Key{}
	for k, c := range codeByKey {
		if o, dup := seen[c]; dup {
			t.Errorf("code 0x%02X used by %s and %s", c, k, o)
		}
		seen[c] = k
	}
}
