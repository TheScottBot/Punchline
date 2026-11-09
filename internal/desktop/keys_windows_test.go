//go:build windows

package desktop

import (
	"testing"

	"github.com/thescottbot/punchline/internal/hotkey"
)

func TestEverySupportedKeyHasAVirtualKey(t *testing.T) {
	for _, k := range hotkey.Supported() {
		if _, ok := vkByKey[k]; !ok {
			t.Errorf("no VK for %s", k)
		}
	}
	if len(vkByKey) != len(hotkey.Supported()) || len(keyByVK) != len(vkByKey) {
		t.Errorf("tables differ: %d %d %d", len(vkByKey), len(keyByVK), len(hotkey.Supported()))
	}
	if vkByKey["F13"] != 0x7C || vkByKey["F24"] != 0x87 {
		t.Error("F13/F24 codes")
	}
}
