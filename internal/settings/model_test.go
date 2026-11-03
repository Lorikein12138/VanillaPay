package settings

import (
	"encoding/json"
	"testing"
)

func TestUnit_HomeHTMLModeRequiresContent(t *testing.T) {
	t.Run("empty_html_is_rejected", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":"","url":null}`)); err == nil {
			t.Fatal("empty HTML content was accepted")
		}
	})

	t.Run("missing_html_is_rejected", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":null,"url":null}`)); err == nil {
			t.Fatal("missing HTML content was accepted")
		}
	})

	t.Run("nonempty_html_is_accepted", func(t *testing.T) {
		if _, _, err := ValidateGroup("home", json.RawMessage(`{"mode":"html","html":"<main>home</main>","url":null}`)); err != nil {
			t.Fatalf("nonempty HTML content was rejected: %v", err)
		}
	})
}
